package discovery_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/serviceconfig"

	disc "github.com/kbukum/gokit/discovery"
	grpccfg "github.com/kbukum/gokit/grpc"
	resdisc "github.com/kbukum/gokit/grpc/resolver/discovery"
	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/security"
)

// ─── Mocks ─────────────────────────────────────────────────────────────────

// fakeDiscovery is an in-memory discovery.Discovery for tests.
type fakeDiscovery struct {
	mu sync.Mutex

	discoverFn func(ctx context.Context, name string) ([]disc.ServiceInstance, error)
	watchFn    func(ctx context.Context, name string) (<-chan []disc.ServiceInstance, error)
	closed     atomic.Bool
}

func (f *fakeDiscovery) Discover(ctx context.Context, name string) ([]disc.ServiceInstance, error) {
	f.mu.Lock()
	fn := f.discoverFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, name)
	}
	return nil, nil
}

func (f *fakeDiscovery) Watch(ctx context.Context, name string) (<-chan []disc.ServiceInstance, error) {
	f.mu.Lock()
	fn := f.watchFn
	f.mu.Unlock()
	if fn != nil {
		return fn(ctx, name)
	}
	// Default: return a closed channel so watch() exits cleanly.
	ch := make(chan []disc.ServiceInstance)
	close(ch)
	return ch, nil
}

func (f *fakeDiscovery) Close() error {
	f.closed.Store(true)
	return nil
}

// recordingCC implements resolver.ClientConn and records callbacks for assertions.
type recordingCC struct {
	mu       sync.Mutex
	states   []resolver.State
	errs     []error
	updateFn func(resolver.State) error // optional override
}

func (c *recordingCC) UpdateState(s resolver.State) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states = append(c.states, s)
	if c.updateFn != nil {
		return c.updateFn(s)
	}
	return nil
}

func (c *recordingCC) ReportError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errs = append(c.errs, err)
}

// Required by resolver.ClientConn but unused in our resolver path.
func (c *recordingCC) NewAddress(_ []resolver.Address) {}
func (c *recordingCC) NewServiceConfig(_ string)       {}
func (c *recordingCC) ParseServiceConfig(_ string) *serviceconfig.ParseResult {
	return &serviceconfig.ParseResult{}
}

func (c *recordingCC) snapshot() ([]resolver.State, []error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := append([]resolver.State(nil), c.states...)
	er := append([]error(nil), c.errs...)
	return st, er
}

// waitFor polls cond every 5ms up to 1s.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", msg)
}

func testLogger() *logger.Logger { return logger.GetGlobalLogger() }

// ─── ResolverBuilder ───────────────────────────────────────────────────────

func TestNewResolverBuilder_DefaultScheme(t *testing.T) {
	b := resdisc.NewResolverBuilder(&fakeDiscovery{}, testLogger())
	if got := b.Scheme(); got != "consul" {
		t.Errorf("scheme: got %q want consul", got)
	}
}

func TestNewResolverBuilder_WithScheme_Override(t *testing.T) {
	b := resdisc.NewResolverBuilder(&fakeDiscovery{}, testLogger(), resdisc.WithScheme("etcd"))
	if got := b.Scheme(); got != "etcd" {
		t.Errorf("scheme: got %q want etcd", got)
	}
}

func TestNewResolverBuilder_NilLogger_FallsBackToGlobal(t *testing.T) {
	// Should not panic when log is nil.
	b := resdisc.NewResolverBuilder(&fakeDiscovery{}, nil)
	if b == nil {
		t.Fatal("builder is nil")
	}
}

// Build wires the resolver, performs the initial resolve synchronously, and
// pushes addresses to the ClientConn.
func TestBuilder_Build_InitialResolveOnSuccess(t *testing.T) {
	fd := &fakeDiscovery{
		discoverFn: func(_ context.Context, name string) ([]disc.ServiceInstance, error) {
			if name != "ssm-ingestion" {
				t.Errorf("Discover got name %q", name)
			}
			return []disc.ServiceInstance{
				{Name: name, Address: "10.0.0.1", Port: 50051},
				{Name: name, Address: "10.0.0.2", Port: 50051},
			}, nil
		},
	}
	cc := &recordingCC{}
	b := resdisc.NewResolverBuilder(fd, testLogger())

	r, err := b.Build(fakeTargetFor("ssm-ingestion"), cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer r.Close()

	states, errs := cc.snapshot()
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 UpdateState call, got %d", len(states))
	}
	if got, want := len(states[0].Addresses), 2; got != want {
		t.Fatalf("addresses: got %d want %d", got, want)
	}
	if states[0].Addresses[0].Addr != "10.0.0.1:50051" {
		t.Errorf("addr[0]: got %q", states[0].Addresses[0].Addr)
	}
	if states[0].Addresses[0].ServerName != "ssm-ingestion" {
		t.Errorf("ServerName: got %q", states[0].Addresses[0].ServerName)
	}
}

// When discovery returns an empty list, the resolver should report
// ErrNoHealthyEndpoints to the ClientConn instead of pushing an empty state.
func TestBuilder_Build_EmptyInstances_ReportsNoHealthy(t *testing.T) {
	fd := &fakeDiscovery{
		discoverFn: func(_ context.Context, _ string) ([]disc.ServiceInstance, error) {
			return nil, nil
		},
	}
	cc := &recordingCC{}
	b := resdisc.NewResolverBuilder(fd, testLogger())

	r, err := b.Build(fakeTargetFor("svc"), cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer r.Close()

	_, errs := cc.snapshot()
	if len(errs) == 0 {
		t.Fatal("expected ReportError call, got none")
	}
	if !errors.Is(errs[0], disc.ErrNoHealthyEndpoints) {
		t.Errorf("error: got %v want wrap of ErrNoHealthyEndpoints", errs[0])
	}
}

// When discovery returns an error, the resolver should propagate via ReportError.
func TestBuilder_Build_DiscoverError_PropagatesViaReportError(t *testing.T) {
	wantErr := errors.New("boom")
	fd := &fakeDiscovery{
		discoverFn: func(_ context.Context, _ string) ([]disc.ServiceInstance, error) {
			return nil, wantErr
		},
	}
	cc := &recordingCC{}
	b := resdisc.NewResolverBuilder(fd, testLogger())

	r, err := b.Build(fakeTargetFor("svc"), cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer r.Close()

	_, errs := cc.snapshot()
	if len(errs) == 0 {
		t.Fatal("expected ReportError, got none")
	}
	if !errors.Is(errs[0], wantErr) {
		t.Errorf("error: got %v want wrap of %v", errs[0], wantErr)
	}
}

// Watch updates should reach UpdateState.
func TestBuilder_Build_WatchUpdatesPropagate(t *testing.T) {
	updates := make(chan []disc.ServiceInstance, 2)
	fd := &fakeDiscovery{
		discoverFn: func(_ context.Context, _ string) ([]disc.ServiceInstance, error) {
			return []disc.ServiceInstance{{Name: "svc", Address: "1.1.1.1", Port: 1}}, nil
		},
		watchFn: func(_ context.Context, _ string) (<-chan []disc.ServiceInstance, error) {
			return updates, nil
		},
	}
	cc := &recordingCC{}
	b := resdisc.NewResolverBuilder(fd, testLogger())

	r, err := b.Build(fakeTargetFor("svc"), cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer r.Close()

	// Initial resolve produced 1 state. Now push a watch update with 2 instances.
	updates <- []disc.ServiceInstance{
		{Name: "svc", Address: "2.2.2.2", Port: 2},
		{Name: "svc", Address: "3.3.3.3", Port: 3},
	}

	waitFor(t, func() bool {
		states, _ := cc.snapshot()
		return len(states) >= 2
	}, "second UpdateState call from watch")

	states, _ := cc.snapshot()
	last := states[len(states)-1]
	if len(last.Addresses) != 2 {
		t.Errorf("addresses: got %d want 2", len(last.Addresses))
	}
	close(updates)
}

// If Watch returns an error, the goroutine should exit cleanly without panic.
func TestBuilder_Build_WatchError_ResolverContinues(t *testing.T) {
	fd := &fakeDiscovery{
		discoverFn: func(_ context.Context, _ string) ([]disc.ServiceInstance, error) {
			return []disc.ServiceInstance{{Name: "svc", Address: "1.1.1.1", Port: 1}}, nil
		},
		watchFn: func(_ context.Context, _ string) (<-chan []disc.ServiceInstance, error) {
			return nil, errors.New("watch unsupported")
		},
	}
	cc := &recordingCC{}
	b := resdisc.NewResolverBuilder(fd, testLogger())

	r, err := b.Build(fakeTargetFor("svc"), cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	r.Close() // must not block / panic
}

// ResolveNow performs an out-of-band re-resolution.
func TestBuilder_ResolveNow_TriggersExtraResolve(t *testing.T) {
	var calls atomic.Int32
	fd := &fakeDiscovery{
		discoverFn: func(_ context.Context, _ string) ([]disc.ServiceInstance, error) {
			calls.Add(1)
			return []disc.ServiceInstance{{Name: "svc", Address: "1.1.1.1", Port: 1}}, nil
		},
	}
	cc := &recordingCC{}
	b := resdisc.NewResolverBuilder(fd, testLogger())

	r, err := b.Build(fakeTargetFor("svc"), cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	defer r.Close()

	initial := calls.Load() // at least 1 from start()

	r.ResolveNow(resolver.ResolveNowOptions{})
	waitFor(t, func() bool { return calls.Load() > initial }, "ResolveNow to trigger Discover")
}

// UpdateState returning an error must NOT crash — it's logged and swallowed.
func TestBuilder_UpdateStateError_IsSwallowed(t *testing.T) {
	fd := &fakeDiscovery{
		discoverFn: func(_ context.Context, _ string) ([]disc.ServiceInstance, error) {
			return []disc.ServiceInstance{{Name: "svc", Address: "1.1.1.1", Port: 1}}, nil
		},
	}
	cc := &recordingCC{updateFn: func(_ resolver.State) error {
		return errors.New("rejected")
	}}
	b := resdisc.NewResolverBuilder(fd, testLogger())

	r, err := b.Build(fakeTargetFor("svc"), cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	r.Close()
}

// Close is idempotent and unblocks the watch goroutine.
func TestBuilder_Close_IsIdempotent(t *testing.T) {
	fd := &fakeDiscovery{}
	b := resdisc.NewResolverBuilder(fd, testLogger())
	r, err := b.Build(fakeTargetFor("svc"), &recordingCC{}, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	r.Close()
	r.Close() // second call must not panic / block
}

// ─── ResolverConnectionFactory ────────────────────────────────────────────

func validGRPCConfig() grpccfg.Config {
	c := grpccfg.Config{
		Addr:    "unused-but-required-by-validate",
		Enabled: true,
	}
	c.ApplyDefaults()
	return c
}

func TestNewResolverConnectionFactory_NewConn_Insecure(t *testing.T) {
	f := resdisc.NewResolverConnectionFactory(&fakeDiscovery{}, validGRPCConfig(), testLogger())
	conn, err := f.NewConn("svc")
	if err != nil {
		t.Fatalf("NewConn: %v", err)
	}
	defer conn.Close()
	if !strings.Contains(conn.Target(), "svc") {
		t.Errorf("target should contain service name, got %q", conn.Target())
	}
}

func TestNewResolverConnectionFactory_NewConn_CustomScheme(t *testing.T) {
	f := resdisc.NewResolverConnectionFactory(
		&fakeDiscovery{},
		validGRPCConfig(),
		testLogger(),
		resdisc.WithScheme("etcd"),
	)
	conn, err := f.NewConn("svc")
	if err != nil {
		t.Fatalf("NewConn: %v", err)
	}
	defer conn.Close()
	if !strings.HasPrefix(conn.Target(), "etcd:///") {
		t.Errorf("target should start with etcd:///, got %q", conn.Target())
	}
}

// Invalid grpc.Config (empty Addr) must surface as error from buildDialOptions.
func TestNewResolverConnectionFactory_InvalidConfig_Errors(t *testing.T) {
	bad := grpccfg.Config{Enabled: true} // Addr empty → ApplyDefaults sets default; force invalid via MaxRecvMsgSize<0
	bad.ApplyDefaults()
	bad.MaxRecvMsgSize = -1 // invalid

	f := resdisc.NewResolverConnectionFactory(&fakeDiscovery{}, bad, testLogger())
	_, err := f.NewConn("svc")
	if err == nil {
		t.Fatal("expected error for invalid config, got nil")
	}
	if !strings.Contains(err.Error(), "grpc config") {
		t.Errorf("error should mention grpc config: %v", err)
	}
}

// TLS configured but invalid path should surface from transportCredentials.
func TestNewResolverConnectionFactory_InvalidTLS_Errors(t *testing.T) {
	cfg := validGRPCConfig()
	cfg.TLS = &security.TLSConfig{
		CertFile: "/no/such/cert.pem",
		KeyFile:  "/no/such/key.pem",
	}

	f := resdisc.NewResolverConnectionFactory(&fakeDiscovery{}, cfg, testLogger())
	_, err := f.NewConn("svc")
	if err == nil {
		t.Fatal("expected TLS error, got nil")
	}
	if !strings.Contains(err.Error(), "tls") && !strings.Contains(err.Error(), "TLS") {
		t.Errorf("error should reference TLS: %v", err)
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────

// fakeTarget builds a resolver.Target whose Endpoint() returns ep.
// gRPC computes Endpoint() from URL.Path with the leading slash stripped, so
// constructing a URL of "scheme:///<ep>" gives us the desired endpoint.
func fakeTargetFor(ep string) resolver.Target {
	u, err := url.Parse("scheme:///" + ep)
	if err != nil {
		panic(err)
	}
	return resolver.Target{URL: *u}
}
