package consul

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"

	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/security"
)

// errUnexpectedTransportCall marks a fake RoundTripper that must never be invoked.
var errUnexpectedTransportCall = errors.New("unexpected transport call")

func TestConfigApplyDefaults(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	cfg.ApplyDefaults()

	if cfg.Addr != "localhost:8500" {
		t.Fatalf("Addr = %q, want localhost:8500", cfg.Addr)
	}
	if cfg.Scheme != "http" || cfg.Datacenter != "dc1" {
		t.Fatalf("Scheme/Datacenter = %q/%q, want http/dc1", cfg.Scheme, cfg.Datacenter)
	}
	if cfg.ConnectTimeout != 10*time.Second || cfg.ReadTimeout != 30*time.Second || cfg.WriteTimeout != 30*time.Second {
		t.Fatalf("timeouts = %v/%v/%v, want 10s/30s/30s", cfg.ConnectTimeout, cfg.ReadTimeout, cfg.WriteTimeout)
	}
	if cfg.Pool == nil {
		t.Fatal("Pool is nil, want defaults")
	}
	if cfg.Pool.MaxIdleConns != 100 || cfg.Pool.MaxIdleConnsPerHost != 10 || cfg.Pool.MaxConnsPerHost != 100 || cfg.Pool.IdleConnTimeout != 90*time.Second {
		t.Fatalf("Pool defaults = %+v, want 100/10/100/90s", cfg.Pool)
	}
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{name: "valid http", cfg: Config{Addr: "localhost:8500", Scheme: "http"}},
		{name: "valid https tls", cfg: Config{Addr: "localhost:8500", Scheme: "https", TLS: &security.TLSConfig{ServerName: "consul.local"}}},
		{name: "missing addr", cfg: Config{Scheme: "http"}, wantErr: true},
		{name: "bad scheme", cfg: Config{Addr: "localhost:8500", Scheme: "ftp"}, wantErr: true},
		{name: "tls without https", cfg: Config{Addr: "localhost:8500", Scheme: "http", TLS: &security.TLSConfig{ServerName: "consul.local"}}, wantErr: true},
		{name: "negative connect timeout", cfg: Config{Addr: "localhost:8500", Scheme: "http", ConnectTimeout: -time.Second}, wantErr: true},
		{name: "negative read timeout", cfg: Config{Addr: "localhost:8500", Scheme: "http", ReadTimeout: -time.Second}, wantErr: true},
		{name: "negative write timeout", cfg: Config{Addr: "localhost:8500", Scheme: "http", WriteTimeout: -time.Second}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("Validate succeeded, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestMergeProviderOptions(t *testing.T) {
	t.Parallel()

	cfg := &Config{Addr: "localhost:8500", Scheme: "http"}
	err := mergeProviderOptions(cfg, map[string]any{
		"datacenter":      "dc-west",
		"namespace":       "payments",
		"connect_timeout": "3s",
		"pool": map[string]any{
			"max_idle_conns":          7,
			"max_idle_conns_per_host": 3,
			"idle_conn_timeout":       "11s",
		},
	})
	if err != nil {
		t.Fatalf("mergeProviderOptions: %v", err)
	}
	if cfg.Datacenter != "dc-west" || cfg.Namespace != "payments" || cfg.ConnectTimeout != 3*time.Second {
		t.Fatalf("merged config = %+v, want datacenter namespace connect_timeout", cfg)
	}
	if cfg.Pool == nil || cfg.Pool.MaxIdleConns != 7 || cfg.Pool.MaxIdleConnsPerHost != 3 || cfg.Pool.IdleConnTimeout != 11*time.Second {
		t.Fatalf("merged pool = %+v, want configured pool", cfg.Pool)
	}

	if err := mergeProviderOptions(&Config{}, map[string]any{"connect_timeout": []string{"bad"}}); err == nil {
		t.Fatal("mergeProviderOptions invalid duration succeeded, want error")
	}
}

func TestNewProviderValidationAndNoNetworkConstruction(t *testing.T) {
	t.Parallel()

	if _, err := NewProvider(discovery.Config{}, &Config{Scheme: "ftp"}, logging.NewDefault("test")); err == nil {
		t.Fatal("NewProvider with invalid config succeeded, want error")
	}

	provider, err := NewProvider(discovery.Config{}, &Config{Addr: "127.0.0.1:8500", Scheme: "http"}, logging.NewDefault("test"))
	if err != nil {
		t.Fatalf("NewProvider valid config: %v", err)
	}
	if provider == nil || provider.client == nil {
		t.Fatal("NewProvider returned nil provider/client")
	}
	if provider.consul.Addr != "127.0.0.1:8500" {
		t.Fatalf("provider consul address = %q, want 127.0.0.1:8500", provider.consul.Addr)
	}
	if err := provider.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestServiceEntryToInstance(t *testing.T) {
	t.Parallel()

	now := time.Unix(123, 0)
	cases := []struct {
		name         string
		entry        *api.ServiceEntry
		wantProtocol string
		wantHealth   discovery.HealthStatus
		wantWeight   int
	}{
		{
			name: "metadata protocol and passing health",
			entry: &api.ServiceEntry{
				Service: &api.AgentService{ID: "api-1", Service: "api", Address: "10.0.0.1", Port: 8080, Tags: []string{"blue"}, Meta: map[string]string{"protocol": "grpc", "weight": "9"}},
				Checks:  api.HealthChecks{{Status: api.HealthPassing}},
			},
			wantProtocol: "grpc",
			wantHealth:   discovery.HealthHealthy,
			wantWeight:   9,
		},
		{
			name: "tag protocol and failing health",
			entry: &api.ServiceEntry{
				Service: &api.AgentService{ID: "api-2", Service: "api", Address: "10.0.0.2", Port: 8081, Tags: []string{"websocket"}, Meta: map[string]string{"weight": "bad"}},
				Checks:  api.HealthChecks{{Status: api.HealthCritical}},
			},
			wantProtocol: "websocket",
			wantHealth:   discovery.HealthUnhealthy,
			wantWeight:   0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := serviceEntryToInstance(tc.entry, now)
			if got.Protocol != tc.wantProtocol || got.Health != tc.wantHealth || got.Weight != tc.wantWeight || !got.LastSeen.Equal(now) {
				t.Fatalf("serviceEntryToInstance = %+v, want protocol=%q health=%q weight=%d lastSeen=%v", got, tc.wantProtocol, tc.wantHealth, tc.wantWeight, now)
			}
		})
	}
}

func TestBuildHealthCheck(t *testing.T) {
	t.Parallel()

	service := &discovery.ServiceInfo{Address: "127.0.0.1", Port: 8080}
	cases := []struct {
		name       string
		healthType string
		scheme     string
		assert     func(*testing.T, *api.AgentServiceCheck)
	}{
		{name: "http default", healthType: discovery.HealthCheckHTTP, scheme: "", assert: func(t *testing.T, check *api.AgentServiceCheck) {
			t.Helper()
			if check.HTTP != "http://127.0.0.1:8080/healthz" {
				t.Fatalf("HTTP = %q, want default http URL", check.HTTP)
			}
		}},
		{name: "grpc https", healthType: discovery.HealthCheckGRPC, scheme: "https", assert: func(t *testing.T, check *api.AgentServiceCheck) {
			t.Helper()
			if check.GRPC != "127.0.0.1:8080" || !check.GRPCUseTLS {
				t.Fatalf("GRPC = %q tls=%v, want address with TLS", check.GRPC, check.GRPCUseTLS)
			}
		}},
		{name: "tcp", healthType: discovery.HealthCheckTCP, scheme: "http", assert: func(t *testing.T, check *api.AgentServiceCheck) {
			t.Helper()
			if check.TCP != "127.0.0.1:8080" {
				t.Fatalf("TCP = %q, want address", check.TCP)
			}
		}},
		{name: "ttl", healthType: discovery.HealthCheckTTL, scheme: "http", assert: func(t *testing.T, check *api.AgentServiceCheck) {
			t.Helper()
			if check.TTL != "10s" || check.Interval != "" || check.Timeout != "" {
				t.Fatalf("TTL/Interval/Timeout = %q/%q/%q, want 10s/empty/empty", check.TTL, check.Interval, check.Timeout)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &Provider{cfg: discovery.Config{Health: discovery.HealthCheckConfig{Type: tc.healthType, Path: "/healthz", Interval: "10s", Timeout: "2s", DeregisterAfter: "1m"}}, consul: &Config{Scheme: tc.scheme}}
			check := provider.buildHealthCheck(service)
			if check.DeregisterCriticalServiceAfter != "1m" {
				t.Fatalf("DeregisterCriticalServiceAfter = %q, want 1m", check.DeregisterCriticalServiceAfter)
			}
			tc.assert(t, check)
		})
	}
}

func TestRegisterDuplicate(t *testing.T) {
	t.Parallel()

	reg := discovery.NewProviderRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, ok := reg.Get("consul"); !ok {
		t.Fatal("consul provider was not registered")
	}
	if err := Register(reg); err == nil {
		t.Fatal("second Register succeeded, want duplicate error")
	}
}

func TestStatsAndClose(t *testing.T) {
	t.Parallel()

	provider := &Provider{}
	if stats := provider.Stats(); stats.RegisteredServices != 0 {
		t.Fatalf("Stats = %+v, want zero", stats)
	}
	if err := provider.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestProvider(t *testing.T, transport http.RoundTripper) *Provider {
	t.Helper()
	client, err := api.NewClient(&api.Config{
		Address:    "consul.test:8500",
		Scheme:     "http",
		HttpClient: &http.Client{Transport: transport},
	})
	if err != nil {
		t.Fatalf("api.NewClient: %v", err)
	}
	return &Provider{
		client: client,
		cfg: discovery.Config{Health: discovery.HealthCheckConfig{
			Type:            discovery.HealthCheckHTTP,
			Path:            "/healthz",
			Interval:        "10s",
			Timeout:         "2s",
			DeregisterAfter: "1m",
		}},
		consul: &Config{Scheme: "http"},
		log:    logging.NewDefault("test"),
	}
}

func TestProviderRegisterDeregisterAndUpdateHealthWithFakeTransport(t *testing.T) {
	t.Parallel()

	var pathsMu sync.Mutex
	paths := make([]string, 0, 4)
	provider := newTestProvider(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			if err := req.Body.Close(); err != nil {
				t.Fatalf("request body close: %v", err)
			}
		}
		pathsMu.Lock()
		paths = append(paths, req.URL.Path)
		pathsMu.Unlock()
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
	}))

	svc := &discovery.ServiceInfo{ID: "api-1", Name: "api", Address: "127.0.0.1", Port: 8080}
	if err := provider.Register(context.Background(), svc); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := provider.Stats().RegisteredServices; got != 1 {
		t.Fatalf("RegisteredServices after Register = %d, want 1", got)
	}
	if err := provider.UpdateHealth(context.Background(), svc.ID, true, "ok"); err != nil {
		t.Fatalf("UpdateHealth healthy: %v", err)
	}
	if err := provider.UpdateHealth(context.Background(), svc.ID, false, "bad"); err != nil {
		t.Fatalf("UpdateHealth unhealthy: %v", err)
	}
	if err := provider.Deregister(context.Background(), svc.ID); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if got := provider.Stats().RegisteredServices; got != 0 {
		t.Fatalf("RegisteredServices after Deregister = %d, want 0", got)
	}

	pathsMu.Lock()
	defer pathsMu.Unlock()
	want := []string{
		"/v1/agent/service/register",
		"/v1/agent/check/pass/service:api-1",
		"/v1/agent/check/fail/service:api-1",
		"/v1/agent/service/deregister/api-1",
	}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths[%d] = %q, want %q (all paths %v)", i, paths[i], want[i], paths)
		}
	}
}

func TestProviderDiscoverWithFakeTransport(t *testing.T) {
	t.Parallel()

	nowEntries := []*api.ServiceEntry{{
		Service: &api.AgentService{ID: "api-1", Service: "api", Address: "10.0.0.1", Port: 8080, Tags: []string{"grpc"}, Meta: map[string]string{"weight": "2"}},
		Checks:  api.HealthChecks{{Status: api.HealthPassing}},
	}}
	body, err := json.Marshal(nowEntries)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	provider := newTestProvider(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/health/service/api" {
			t.Fatalf("path = %q, want /v1/health/service/api", req.URL.Path)
		}
		if req.URL.Query().Get("passing") != "1" {
			t.Fatalf("passing query = %q, want 1", req.URL.Query().Get("passing"))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: req}, nil
	}))

	instances, err := provider.Discover(context.Background(), "api")
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(instances) != 1 || instances[0].ID != "api-1" || instances[0].Protocol != "grpc" || instances[0].Weight != 2 {
		t.Fatalf("Discover returned %+v, want mapped api-1 instance", instances)
	}
}

func TestProviderDiscoverNoHealthyAndTransportErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		response  *http.Response
		roundErr  error
		wantError error
	}{
		{name: "empty response", response: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("[]"))}, wantError: discovery.ErrNoHealthyEndpoints},
		{name: "transport error", roundErr: errors.New("dial failed")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := newTestProvider(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if tc.roundErr != nil {
					return nil, tc.roundErr
				}
				tc.response.Request = req
				return tc.response, nil
			}))
			_, err := provider.Discover(context.Background(), "api")
			if err == nil {
				t.Fatal("Discover succeeded, want error")
			}
			if tc.wantError != nil && !errors.Is(err, tc.wantError) {
				t.Fatalf("Discover error = %v, want %v", err, tc.wantError)
			}
		})
	}
}

func TestProviderRegisterWrapsTransportError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("consul down")
	provider := newTestProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantErr
	}))
	if err := provider.Register(context.Background(), &discovery.ServiceInfo{ID: "api-1", Name: "api", Address: "127.0.0.1", Port: 8080}); !errors.Is(err, wantErr) {
		t.Fatalf("Register error = %v, want wrapped transport error", err)
	}
}

func TestRegisterFactoryBuildsProviderWithOptions(t *testing.T) {
	t.Parallel()

	reg := discovery.NewProviderRegistry()
	if err := Register(reg); err != nil {
		t.Fatalf("Register: %v", err)
	}
	factory, ok := reg.Get("consul")
	if !ok {
		t.Fatal("consul provider missing")
	}
	registry, disc, err := factory(discovery.Config{
		Addr:   "127.0.0.1:8500",
		Scheme: "http",
		ProviderOptions: map[string]any{
			"datacenter":      "dc-test",
			"connect_timeout": "4s",
		},
	}, logging.NewDefault("test"))
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if registry == nil || disc == nil {
		t.Fatal("factory returned nil registry/discovery")
	}
	provider, ok := registry.(*Provider)
	if !ok {
		t.Fatalf("registry type = %T, want *Provider", registry)
	}
	if provider.consul.Datacenter != "dc-test" || provider.consul.ConnectTimeout != 4*time.Second {
		t.Fatalf("provider consul config = %+v, want provider options merged", provider.consul)
	}
}

func TestNewProviderAppliesTLSAndPoolOptions(t *testing.T) {
	t.Parallel()

	provider, err := NewProvider(discovery.Config{}, &Config{
		Addr:   "127.0.0.1:8500",
		Scheme: "https",
		TLS:    &security.TLSConfig{ServerName: "consul.local", SkipVerify: true},
		Pool:   &PoolConfig{MaxIdleConns: 3, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 1, IdleConnTimeout: 5 * time.Second},
	}, logging.NewDefault("test"))
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	if provider == nil || provider.client == nil {
		t.Fatal("NewProvider returned nil provider/client")
	}
}

func TestWatchWithCanceledContextClosesWithoutNetwork(t *testing.T) {
	t.Parallel()

	provider := newTestProvider(t, roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("transport should not be called for pre-canceled watch context")
		return nil, errUnexpectedTransportCall
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch, err := provider.Watch(ctx, "api")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("watch channel open, want closed")
		}
	case <-time.After(time.Second):
		t.Fatal("watch channel did not close after context cancellation")
	}
}
