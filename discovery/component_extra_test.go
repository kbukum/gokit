package discovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
)

type componentFakeProvider struct {
	instances    map[string][]ServiceInstance
	registerErr  error
	deregistered []string
	closed       bool
	registered   []*ServiceInfo
	stats        RegistryStats
}

func (f *componentFakeProvider) Register(_ context.Context, svc *ServiceInfo) error {
	if f.registerErr != nil {
		return f.registerErr
	}
	f.registered = append(f.registered, svc)
	f.stats.RegisteredServices++
	return nil
}

func (f *componentFakeProvider) Deregister(_ context.Context, serviceID string) error {
	f.deregistered = append(f.deregistered, serviceID)
	return nil
}

func (f *componentFakeProvider) UpdateHealth(context.Context, string, bool, string) error { return nil }
func (f *componentFakeProvider) Stats() RegistryStats                                     { return f.stats }
func (f *componentFakeProvider) Close() error                                             { f.closed = true; return nil }
func (f *componentFakeProvider) Discover(_ context.Context, name string) ([]ServiceInstance, error) {
	instances := f.instances[name]
	if len(instances) == 0 {
		return nil, ErrServiceNotFound
	}
	return instances, nil
}

func (f *componentFakeProvider) Watch(ctx context.Context, _ string) (<-chan []ServiceInstance, error) {
	ch := make(chan []ServiceInstance)
	go func() { <-ctx.Done(); close(ch) }()
	return ch, nil
}

func TestProviderRegistryGetAndValidation(t *testing.T) {
	t.Parallel()

	reg := NewProviderRegistry()
	if _, ok := reg.Get("missing"); ok {
		t.Fatal("Get missing provider ok = true, want false")
	}
	if err := reg.Register("", func(Config, *logging.Logger) (Registry, Discovery, error) { return nil, nil, nil }); err == nil {
		t.Fatal("Register empty name succeeded, want error")
	}
	if err := reg.Register("nil", nil); err == nil {
		t.Fatal("Register nil factory succeeded, want error")
	}
}

func TestComponentLifecycleWithStaticProvider(t *testing.T) {
	t.Parallel()

	fake := &componentFakeProvider{instances: map[string][]ServiceInstance{
		"dep": {{ID: "dep-1", Name: "dep", Address: "10.0.0.9", Port: 9090, Health: HealthHealthy}},
	}}
	reg := NewProviderRegistry()
	if err := reg.Register("static", func(Config, *logging.Logger) (Registry, Discovery, error) { return fake, fake, nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}

	comp, err := NewComponent(reg, Config{Provider: "static", Services: []DiscoveredService{{Name: "dep"}}}, testLogger(), WithIPProbeTarget("192.0.2.1:9"))
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	if comp.Name() != "discovery" {
		t.Fatalf("Name = %q, want discovery", comp.Name())
	}
	if comp.Registry() != nil || comp.Discovery() != nil || comp.Client() != nil {
		t.Fatal("new component should not expose registry/discovery/client before Start")
	}
	if health := comp.Health(context.Background()); health.Status != component.StatusUnhealthy {
		t.Fatalf("Health before Start = %s, want unhealthy", health.Status)
	}

	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if comp.Registry() == nil || comp.Discovery() == nil || comp.Client() == nil {
		t.Fatal("started component did not expose registry/discovery/client")
	}
	if health := comp.Health(context.Background()); health.Status != component.StatusHealthy || health.Message != "disabled (static)" {
		t.Fatalf("Health disabled = %+v, want healthy static", health)
	}
	desc := comp.Describe()
	if desc.Name != "Discovery" || desc.Type != "discovery" {
		t.Fatalf("Describe = %+v, want discovery description", desc)
	}
	if err := comp.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !fake.closed {
		t.Fatal("Stop did not close discovery provider")
	}
}

func TestComponentEnabledRegistrationAndHealth(t *testing.T) {
	t.Parallel()

	fake := &componentFakeProvider{instances: map[string][]ServiceInstance{}, stats: RegistryStats{RegisteredServices: 1}}
	reg := NewProviderRegistry()
	if err := reg.Register("fake", func(Config, *logging.Logger) (Registry, Discovery, error) { return fake, fake, nil }); err != nil {
		t.Fatalf("Register: %v", err)
	}
	cfg := Config{
		Enabled:  true,
		Provider: "fake",
		Registration: RegistrationConfig{
			Enabled:        true,
			Required:       true,
			MaxRetries:     1,
			ServiceName:    "api",
			ServiceID:      "api-1",
			ServiceAddress: "127.0.0.1",
			ServicePort:    8080,
		},
	}
	comp, err := NewComponent(reg, cfg, testLogger())
	if err != nil {
		t.Fatalf("NewComponent: %v", err)
	}
	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(fake.registered) != 1 || fake.registered[0].ID != "api-1" {
		t.Fatalf("registered services = %+v, want api-1", fake.registered)
	}
	if health := comp.Health(context.Background()); health.Status != component.StatusHealthy {
		t.Fatalf("Health = %+v, want healthy", health)
	}
	if err := comp.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(fake.deregistered) != 1 || fake.deregistered[0] != "api-1" {
		t.Fatalf("deregistered = %v, want [api-1]", fake.deregistered)
	}
}

func TestComponentStartErrorPaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		reg  *ProviderRegistry
		cfg  Config
	}{
		{name: "nil registry"},
		{name: "missing static", reg: NewProviderRegistry(), cfg: Config{}},
		{name: "invalid config", reg: NewProviderRegistry(), cfg: Config{Enabled: true, Provider: "missing"}},
		{name: "unsupported provider", reg: NewProviderRegistry(), cfg: Config{Enabled: true, Provider: "missing", Registration: RegistrationConfig{ServiceName: "api", ServicePort: 8080}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			comp, err := NewComponent(tc.reg, tc.cfg, testLogger())
			if tc.reg == nil {
				if err == nil || comp != nil {
					t.Fatalf("NewComponent nil registry = (%v, %v), want error", comp, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewComponent: %v", err)
			}
			if err := comp.Start(context.Background()); err == nil {
				t.Fatal("Start succeeded, want error")
			}
		})
	}
}

func TestConfigRetryConfigAndResolveAddr(t *testing.T) {
	t.Parallel()

	retry := RegistrationConfig{MaxRetries: 4, RetryInterval: "250ms"}.RetryConfig()
	if retry.MaxAttempts != 4 || retry.InitialBackoff != 250*time.Millisecond || retry.MaxBackoff != 2*time.Second {
		t.Fatalf("RetryConfig = %+v, want attempts=4 initial=250ms max=2s", retry)
	}
	fallbackRetry := RegistrationConfig{MaxRetries: 2, RetryInterval: "bad"}.RetryConfig()
	if fallbackRetry.InitialBackoff != 2*time.Second {
		t.Fatalf("RetryConfig invalid interval initial = %v, want 2s", fallbackRetry.InitialBackoff)
	}

	mock := &mockDiscovery{instances: map[string][]ServiceInstance{"db": {makeInstance("db-1", "db", "127.0.0.1", 5432)}}}
	host, port, err := ResolveAddr(mock, "db")
	if err != nil {
		t.Fatalf("ResolveAddr: %v", err)
	}
	if host != "127.0.0.1" || port != 5432 {
		t.Fatalf("ResolveAddr = %s:%d, want 127.0.0.1:5432", host, port)
	}
	mock.err = errors.New("backend down")
	if _, _, err := ResolveAddr(mock, "db"); err == nil {
		t.Fatal("ResolveAddr backend error succeeded, want error")
	}
	mock.err = nil
	if _, _, err := ResolveAddr(mock, "missing"); err == nil {
		t.Fatal("ResolveAddr missing service succeeded, want error")
	}
}

func TestClientAdditionalBranches(t *testing.T) {
	t.Parallel()

	httpViaTag := makeInstance("tag-http", "svc", "10.0.0.1", 80)
	httpViaTag.Protocol = ""
	httpViaTag.Tags = []string{"protocol:http"}
	mock := &mockDiscovery{instances: map[string][]ServiceInstance{"svc": {httpViaTag}}}
	client := NewClient(mock, ClientConfig{CacheTTL: time.Minute, StaticEndpoints: []StaticEndpoint{{Name: "fb", Address: "", Port: 8080, Healthy: false}}}, testLogger())
	defer func() {
		if err := client.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()
	got, err := client.DiscoverOne(context.Background(), Query{ServiceName: "svc", Protocol: "http", Strategy: LeastConn})
	if err != nil {
		t.Fatalf("DiscoverOne: %v", err)
	}
	if got.ID != "tag-http" {
		t.Fatalf("DiscoverOne = %q, want tag-http", got.ID)
	}

	required := NewClient(&mockDiscovery{err: errors.New("down")}, ClientConfig{Services: []string{"must"}, Criticality: map[string]Criticality{"must": CriticalityRequired}}, testLogger())
	defer required.Close()
	if _, err := required.DiscoverAll(context.Background()); err == nil {
		t.Fatal("DiscoverAll required failure succeeded, want error")
	}
}

func TestServiceInstanceHostPort(t *testing.T) {
	t.Parallel()

	inst := ServiceInstance{Address: "127.0.0.1", Port: 8080}
	if got := inst.HostPort(); got != "127.0.0.1:8080" {
		t.Fatalf("HostPort = %q, want 127.0.0.1:8080", got)
	}
}
