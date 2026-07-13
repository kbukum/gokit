package provider_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/provider"
)

// testProvider implements the Provider interface for testing.
type testProvider struct {
	name      string
	available bool
}

func (p *testProvider) Name() string                         { return p.name }
func (p *testProvider) IsAvailable(ctx context.Context) bool { return p.available }

func TestRegistryRegisterAndCreate(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	reg.RegisterFactory("test", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "test", available: true}, nil
	})

	p, err := reg.Create("test", nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if p.Name() != "test" {
		t.Errorf("expected name 'test', got %q", p.Name())
	}
}

func TestRegistryCreateUnregistered(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	_, err := reg.Create("missing", nil)
	if err == nil {
		t.Error("expected error for unregistered factory")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' in error, got %q", err.Error())
	}
}

func TestRegistryList(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	reg.RegisterFactory("beta", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "beta"}, nil
	})
	reg.RegisterFactory("alpha", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "alpha"}, nil
	})

	names := reg.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("expected sorted [alpha, beta], got %v", names)
	}
}

func TestRegistryGetSet(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	p := &testProvider{name: "cached", available: true}

	_, ok := reg.Get("cached")
	if ok {
		t.Error("expected Get to return false before Set")
	}

	reg.Set("cached", p)
	got, ok := reg.Get("cached")
	if !ok {
		t.Fatal("expected Get to return true after Set")
	}
	if got.Name() != "cached" {
		t.Errorf("expected 'cached', got %q", got.Name())
	}
}

func TestPrioritySelector(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"primary":   {name: "primary", available: false},
		"secondary": {name: "secondary", available: true},
		"tertiary":  {name: "tertiary", available: true},
	}

	sel := &provider.PrioritySelector[*testProvider]{
		Priority: []string{"primary", "secondary", "tertiary"},
	}

	p, err := sel.Select(ctx, providers)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if p.Name() != "secondary" {
		t.Errorf("expected 'secondary' (first available), got %q", p.Name())
	}
}

func TestPrioritySelectorNoneAvailable(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"a": {name: "a", available: false},
	}

	sel := &provider.PrioritySelector[*testProvider]{Priority: []string{"a"}}
	_, err := sel.Select(ctx, providers)
	if err == nil {
		t.Error("expected error when no provider is available")
	}
}

func TestRoundRobinSelector(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"a": {name: "a", available: true},
		"b": {name: "b", available: true},
	}

	sel := &provider.RoundRobinSelector[*testProvider]{}

	// Call multiple times to verify round-robin behavior
	seen := map[string]int{}
	for i := 0; i < 10; i++ {
		p, err := sel.Select(ctx, providers)
		if err != nil {
			t.Fatalf("Select failed: %v", err)
		}
		seen[p.Name()]++
	}

	if len(seen) != 2 {
		t.Errorf("expected 2 different providers, got %d", len(seen))
	}
	if seen["a"] == 0 || seen["b"] == 0 {
		t.Errorf("expected both providers selected, got %v", seen)
	}
}

func TestRoundRobinSelectorEmpty(t *testing.T) {
	ctx := context.Background()
	sel := &provider.RoundRobinSelector[*testProvider]{}
	_, err := sel.Select(ctx, map[string]*testProvider{})
	if err == nil {
		t.Error("expected error for empty providers")
	}
}

func TestHealthCheckSelector(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"a": {name: "a", available: false},
		"b": {name: "b", available: true},
	}

	sel := &provider.HealthCheckSelector[*testProvider]{}
	p, err := sel.Select(ctx, providers)
	if err != nil {
		t.Fatalf("Select failed: %v", err)
	}
	if p.Name() != "b" {
		t.Errorf("expected 'b' (available), got %q", p.Name())
	}
}

func TestHealthCheckSelectorNoneAvailable(t *testing.T) {
	ctx := context.Background()
	providers := map[string]*testProvider{
		"a": {name: "a", available: false},
	}

	sel := &provider.HealthCheckSelector[*testProvider]{}
	_, err := sel.Select(ctx, providers)
	if err == nil {
		t.Error("expected error when no provider is available")
	}
}

func TestManagerInitializeAndGet(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	sel := &provider.PrioritySelector[*testProvider]{Priority: []string{"main"}}
	mgr := provider.NewManager[*testProvider](reg, sel)

	mgr.Register("main", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "main", available: true}, nil
	})

	if err := mgr.Initialize("main", nil); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	ctx := context.Background()
	p, err := mgr.Get(ctx)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if p.Name() != "main" {
		t.Errorf("expected 'main', got %q", p.Name())
	}
}

func TestManagerGetByName(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	sel := &provider.PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := provider.NewManager[*testProvider](reg, sel)

	mgr.Register("svc", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "svc", available: true}, nil
	})
	mgr.Initialize("svc", nil)

	p, err := mgr.GetByName("svc")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if p.Name() != "svc" {
		t.Errorf("expected 'svc', got %q", p.Name())
	}
}

func TestManagerGetByNameNotFound(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	sel := &provider.PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := provider.NewManager[*testProvider](reg, sel)

	_, err := mgr.GetByName("missing")
	if err == nil {
		t.Error("expected error for missing provider")
	}
}

func TestManagerSetDefault(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	sel := &provider.PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := provider.NewManager[*testProvider](reg, sel)

	mgr.Register("a", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "a", available: true}, nil
	})
	mgr.Register("b", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "b", available: true}, nil
	})
	mgr.Initialize("a", nil)
	mgr.Initialize("b", nil)

	if err := mgr.SetDefault("b"); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	ctx := context.Background()
	p, err := mgr.Get(ctx)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if p.Name() != "b" {
		t.Errorf("expected default 'b', got %q", p.Name())
	}
}

func TestManagerSetDefaultNotInitialized(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	sel := &provider.PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := provider.NewManager[*testProvider](reg, sel)

	err := mgr.SetDefault("missing")
	if err == nil {
		t.Error("expected error for setting default to uninitialized provider")
	}
}

func TestManagerAvailable(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	sel := &provider.PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := provider.NewManager[*testProvider](reg, sel)

	mgr.Register("x", func(cfg map[string]any) (*testProvider, error) {
		return &testProvider{name: "x", available: true}, nil
	})
	mgr.Initialize("x", nil)

	avail := mgr.Available()
	if len(avail) != 1 {
		t.Fatalf("expected 1 available, got %d", len(avail))
	}
	if avail[0] != "x" {
		t.Errorf("expected 'x', got %q", avail[0])
	}
}

func TestManagerInitializeFailure(t *testing.T) {
	reg := provider.NewRegistry[*testProvider]()
	sel := &provider.PrioritySelector[*testProvider]{Priority: []string{}}
	mgr := provider.NewManager[*testProvider](reg, sel)

	err := mgr.Initialize("unregistered", nil)
	if err == nil {
		t.Error("expected error for initializing unregistered provider")
	}
}

type countingIterator[T any] struct {
	items    []T
	pos      int
	closeErr error
}

func (it *countingIterator[T]) Next(_ context.Context) (val T, ok bool, err error) {
	if it.pos >= len(it.items) {
		var zero T
		return zero, false, nil
	}
	v := it.items[it.pos]
	it.pos++
	return v, true, nil
}
func (it *countingIterator[T]) Close() error { return it.closeErr }

// errorAtNIterator returns an error after N successful items.
type errorAtNIterator struct {
	items []int
	pos   int
	errAt int
}

func (it *errorAtNIterator) Next(_ context.Context) (val int, ok bool, err error) {
	if it.pos >= len(it.items) {
		return 0, false, nil
	}
	if it.pos == it.errAt {
		return 0, false, errors.New("iterator error")
	}
	v := it.items[it.pos]
	it.pos++
	return v, true, nil
}
func (it *errorAtNIterator) Close() error { return nil }

// blockingIterator blocks until context is canceled, useful for concurrency tests.
type blockingIterator struct {
	items    []int
	pos      int
	blockAt  int
	unblock  chan struct{}
	closeMu  sync.Mutex
	closed   bool
	closeErr error
}

func (it *blockingIterator) Next(ctx context.Context) (val int, ok bool, err error) {
	if it.pos >= len(it.items) {
		return 0, false, nil
	}
	if it.pos == it.blockAt {
		select {
		case <-it.unblock:
		case <-ctx.Done():
			return 0, false, ctx.Err()
		}
	}
	v := it.items[it.pos]
	it.pos++
	return v, true, nil
}
func (it *blockingIterator) Close() error {
	it.closeMu.Lock()
	defer it.closeMu.Unlock()
	it.closed = true
	return it.closeErr
}

// controlledDuplex is a Duplex provider that can simulate errors.
type controlledDuplex struct {
	name      string
	available bool
	openErr   error
	stream    *controlledDuplexStream
}

func (d *controlledDuplex) Name() string                       { return d.name }
func (d *controlledDuplex) IsAvailable(_ context.Context) bool { return d.available }
func (d *controlledDuplex) Open(_ context.Context) (provider.DuplexStream[string, string], error) {
	if d.openErr != nil {
		return nil, d.openErr
	}
	return d.stream, nil
}

type controlledDuplexStream struct {
	mu       sync.Mutex
	sendErr  error
	recvCh   chan string
	closed   bool
	closeErr error
}

func newControlledDuplexStream() *controlledDuplexStream {
	return &controlledDuplexStream{
		recvCh: make(chan string, 10),
	}
}
func (s *controlledDuplexStream) Send(in string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("stream closed")
	}
	if s.sendErr != nil {
		return s.sendErr
	}
	s.recvCh <- "echo:" + in
	return nil
}
func (s *controlledDuplexStream) Recv() (string, error) {
	v, ok := <-s.recvCh
	if !ok {
		return "", io.EOF
	}
	return v, nil
}
func (s *controlledDuplexStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.recvCh)
	}
	return s.closeErr
}

// initErrProvider is Initializable but returns an error from Init.
type initErrProvider struct {
	name string
}

func (p *initErrProvider) Name() string                                         { return p.name }
func (p *initErrProvider) IsAvailable(_ context.Context) bool                   { return true }
func (p *initErrProvider) Execute(_ context.Context, in string) (string, error) { return in, nil }
func (p *initErrProvider) Init(_ context.Context) error                         { return errors.New("init failed") }

// nonCloseableProvider does NOT implement Closeable.
type nonCloseableProvider struct {
	name string
}

func (p *nonCloseableProvider) Name() string                                         { return p.name }
func (p *nonCloseableProvider) IsAvailable(_ context.Context) bool                   { return true }
func (p *nonCloseableProvider) Execute(_ context.Context, in string) (string, error) { return in, nil }

// closeErrProvider implements Closeable but returns an error from Close.
type closeErrProvider struct {
	name string
}

func (p *closeErrProvider) Name() string                                         { return p.name }
func (p *closeErrProvider) IsAvailable(_ context.Context) bool                   { return true }
func (p *closeErrProvider) Execute(_ context.Context, in string) (string, error) { return in, nil }
func (p *closeErrProvider) Close(_ context.Context) error                        { return errors.New("close error") }

// healthCheckProvider implements HealthChecker.
type healthCheckProvider struct {
	name   string
	status provider.HealthStatus
}

func (p *healthCheckProvider) Name() string { return p.name }
func (p *healthCheckProvider) IsAvailable(_ context.Context) bool {
	return p.status.Status == provider.StatusHealthy
}
func (p *healthCheckProvider) Execute(_ context.Context, in string) (string, error) {
	return in, nil
}
func (p *healthCheckProvider) Health(_ context.Context) provider.HealthStatus {
	return p.status
}

type slowInitProvider struct {
	name      string
	initDelay time.Duration
}

func (p *slowInitProvider) Name() string                       { return p.name }
func (p *slowInitProvider) IsAvailable(_ context.Context) bool { return true }
func (p *slowInitProvider) Execute(_ context.Context, in string) (string, error) {
	return in, nil
}
func (p *slowInitProvider) Init(ctx context.Context) error {
	select {
	case <-time.After(p.initDelay):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type closeTrackingProvider struct {
	name        string
	closeCalled *bool
}

func (p *closeTrackingProvider) Name() string                                         { return p.name }
func (p *closeTrackingProvider) IsAvailable(_ context.Context) bool                   { return true }
func (p *closeTrackingProvider) Execute(_ context.Context, in string) (string, error) { return in, nil }
func (p *closeTrackingProvider) Close(_ context.Context) error {
	*p.closeCalled = true
	return nil
}

type slowHealthProvider struct {
	name  string
	delay time.Duration
}

func (p *slowHealthProvider) Name() string                       { return p.name }
func (p *slowHealthProvider) IsAvailable(_ context.Context) bool { return true }
func (p *slowHealthProvider) Execute(_ context.Context, in string) (string, error) {
	return in, nil
}
func (p *slowHealthProvider) Health(ctx context.Context) provider.HealthStatus {
	select {
	case <-time.After(p.delay):
		return provider.HealthStatus{Status: provider.StatusHealthy, Message: "ok"}
	case <-ctx.Done():
		return provider.HealthStatus{Status: provider.StatusUnavailable, Message: "health check timed out"}
	}
}

func TestEmptyProviderName(t *testing.T) {
	t.Parallel()
	p := &echoProvider{name: ""}
	if p.Name() != "" {
		t.Fatalf("expected empty name, got %q", p.Name())
	}

	// Empty name should work with registry
	registry := provider.NewRegistry[provider.RequestResponse[string, string]]()
	registry.RegisterFactory("", func(_ map[string]any) (provider.RequestResponse[string, string], error) {
		return p, nil
	})
	created, err := registry.Create("", nil)
	if err != nil {
		t.Fatalf("Create with empty name: %v", err)
	}
	if created.Name() != "" {
		t.Fatalf("expected empty name, got %q", created.Name())
	}
}
