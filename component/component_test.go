package component

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockComponent implements Component for testing.
type mockComponent struct {
	name       string
	startErr   error
	stopErr    error
	health     Health
	startOrder *[]string
	stopOrder  *[]string
}

func (m *mockComponent) Name() string { return m.name }
func (m *mockComponent) Start(ctx context.Context) error {
	if m.startOrder != nil {
		*m.startOrder = append(*m.startOrder, m.name)
	}
	return m.startErr
}

func (m *mockComponent) Stop(ctx context.Context) error {
	if m.stopOrder != nil {
		*m.stopOrder = append(*m.stopOrder, m.name)
	}
	return m.stopErr
}

func (m *mockComponent) Health(ctx context.Context) Health {
	return m.health
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestRegister(t *testing.T) {
	r := NewRegistry()
	c := &mockComponent{name: "db", health: Health{Name: "db", Status: StatusHealthy}}

	if err := r.Register(c); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
}

func TestRegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	c := &mockComponent{name: "db"}
	r.Register(c)

	err := r.Register(&mockComponent{name: "db"})
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestGet(t *testing.T) {
	r := NewRegistry()
	c := &mockComponent{name: "db"}
	r.Register(c)

	got := r.Get("db")
	if got == nil {
		t.Fatal("expected to get registered component")
	}
	if got.Name() != "db" {
		t.Errorf("expected 'db', got %q", got.Name())
	}
}

func TestGetNotFound(t *testing.T) {
	r := NewRegistry()
	got := r.Get("missing")
	if got != nil {
		t.Error("expected nil for unregistered component")
	}
}

func TestStartAll(t *testing.T) {
	r := NewRegistry()
	order := []string{}

	r.Register(&mockComponent{
		name: "db", startOrder: &order,
		health: Health{Name: "db", Status: StatusHealthy},
	})
	r.Register(&mockComponent{
		name: "cache", startOrder: &order,
		health: Health{Name: "cache", Status: StatusHealthy},
	})

	if err := r.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}

	if len(order) != 2 {
		t.Fatalf("expected 2 starts, got %d", len(order))
	}
	if order[0] != "db" || order[1] != "cache" {
		t.Errorf("expected start order [db, cache], got %v", order)
	}
}

func TestStartAllError(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockComponent{name: "db", startErr: fmt.Errorf("connection refused")})

	err := r.StartAll(context.Background())
	if err == nil {
		t.Error("expected error from StartAll")
	}
}

func TestStopAllReverseOrder(t *testing.T) {
	r := NewRegistry()
	order := []string{}

	r.Register(&mockComponent{name: "db", stopOrder: &order, health: Health{Name: "db", Status: StatusHealthy}})
	r.Register(&mockComponent{name: "cache", stopOrder: &order, health: Health{Name: "cache", Status: StatusHealthy}})
	r.Register(&mockComponent{name: "kafka", stopOrder: &order, health: Health{Name: "kafka", Status: StatusHealthy}})

	r.StartAll(context.Background())
	if err := r.StopAll(context.Background()); err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 stops, got %d", len(order))
	}
	if order[0] != "kafka" || order[1] != "cache" || order[2] != "db" {
		t.Errorf("expected reverse stop order [kafka, cache, db], got %v", order)
	}
}

func TestStopAllSkipsUnstarted(t *testing.T) {
	r := NewRegistry()
	order := []string{}
	r.Register(&mockComponent{name: "db", stopOrder: &order})

	// Don't start, then stop
	if err := r.StopAll(context.Background()); err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}
	if len(order) != 0 {
		t.Errorf("expected 0 stops for unstarted components, got %d", len(order))
	}
}

func TestStopAllWithErrors(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockComponent{
		name: "db", stopErr: fmt.Errorf("stop failed"),
		health: Health{Name: "db", Status: StatusHealthy},
	})
	r.StartAll(context.Background())

	err := r.StopAll(context.Background())
	if err == nil {
		t.Error("expected error from StopAll")
	}
}

func TestHealthAll(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockComponent{
		name:   "db",
		health: Health{Name: "db", Status: StatusHealthy, Message: "connected"},
	})
	r.Register(&mockComponent{
		name:   "cache",
		health: Health{Name: "cache", Status: StatusUnhealthy, Message: "timeout"},
	})

	results := r.HealthAll(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != StatusHealthy {
		t.Errorf("expected db healthy, got %s", results[0].Status)
	}
	if results[1].Status != StatusUnhealthy {
		t.Errorf("expected cache unhealthy, got %s", results[1].Status)
	}
}

func TestHealthStatusConstants(t *testing.T) {
	if StatusHealthy != "healthy" {
		t.Errorf("expected 'healthy', got %q", StatusHealthy)
	}
	if StatusUnhealthy != "unhealthy" {
		t.Errorf("expected 'unhealthy', got %q", StatusUnhealthy)
	}
	if StatusDegraded != "degraded" {
		t.Errorf("expected 'degraded', got %q", StatusDegraded)
	}
}

func TestBaseLazyComponent(t *testing.T) {
	initialized := false
	lc := NewBaseLazyComponent("lazy-db", func(ctx context.Context) error {
		initialized = true
		return nil
	})

	if lc.Name() != "lazy-db" {
		t.Errorf("expected name 'lazy-db', got %q", lc.Name())
	}
	if lc.IsInitialized() {
		t.Error("expected not initialized before Initialize()")
	}

	if err := lc.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if !initialized {
		t.Error("expected initializer to be called")
	}
	if !lc.IsInitialized() {
		t.Error("expected IsInitialized() to return true after init")
	}
}

func TestBaseLazyComponentDoubleInit(t *testing.T) {
	count := 0
	lc := NewBaseLazyComponent("svc", func(ctx context.Context) error {
		count++
		return nil
	})

	lc.Initialize(context.Background())
	lc.Initialize(context.Background())
	if count != 1 {
		t.Errorf("expected initializer called once, got %d", count)
	}
}

func TestBaseLazyHealthCheck(t *testing.T) {
	lc := NewBaseLazyComponent("svc", func(ctx context.Context) error { return nil })

	// Not initialized yet
	err := lc.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected error for health check on uninitialized component")
	}

	lc.Initialize(context.Background())
	if err := lc.HealthCheck(context.Background()); err != nil {
		t.Errorf("expected nil after init, got %v", err)
	}
}

func TestBaseLazyComponentWithHealthCheck(t *testing.T) {
	lc := NewBaseLazyComponent("svc", func(ctx context.Context) error { return nil })
	lc.WithHealthCheck(func(ctx context.Context) error {
		return fmt.Errorf("degraded")
	})

	lc.Initialize(context.Background())
	err := lc.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected custom health check error")
	}
}

func TestBaseLazyComponentClose(t *testing.T) {
	closed := false
	lc := NewBaseLazyComponent("svc", func(ctx context.Context) error { return nil })
	lc.WithCloser(func() error {
		closed = true
		return nil
	})

	lc.Initialize(context.Background())
	if err := lc.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !closed {
		t.Error("expected closer to be called")
	}
	if lc.IsInitialized() {
		t.Error("expected not initialized after close")
	}
}

// ---------------------------------------------------------------------------
// Additional mock types for new tests
// ---------------------------------------------------------------------------

// slowComponent blocks Start until context is done or duration elapses.
type slowComponent struct {
	name       string
	delay      time.Duration
	startOrder *[]string
	stopOrder  *[]string
	started    bool
}

func (s *slowComponent) Name() string { return s.name }
func (s *slowComponent) Start(ctx context.Context) error {
	if s.startOrder != nil {
		*s.startOrder = append(*s.startOrder, s.name)
	}
	select {
	case <-time.After(s.delay):
		s.started = true
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *slowComponent) Stop(ctx context.Context) error {
	if s.stopOrder != nil {
		*s.stopOrder = append(*s.stopOrder, s.name)
	}
	select {
	case <-time.After(s.delay):
		s.started = false
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *slowComponent) Health(ctx context.Context) Health {
	return Health{Name: s.name, Status: StatusHealthy}
}

// describableComponent implements both Component and Describable.
type describableComponent struct {
	mockComponent
	desc Description
}

func (d *describableComponent) Describe() Description { return d.desc }

// routeProviderComponent implements Component and RouteProvider.
type routeProviderComponent struct {
	mockComponent
	routes []Route
}

func (rp *routeProviderComponent) Routes() []Route { return rp.routes }

// ---------------------------------------------------------------------------
// GAP 1: Concurrent registration
// ---------------------------------------------------------------------------

func TestConcurrentRegistration(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	const n = 50
	var wg sync.WaitGroup
	errCh := make(chan error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := &mockComponent{
				name:   fmt.Sprintf("comp-%d", i),
				health: Health{Name: fmt.Sprintf("comp-%d", i), Status: StatusHealthy},
			}
			if err := r.Register(c); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("unexpected registration error: %v", err)
	}

	all := r.All()
	if len(all) != n {
		t.Errorf("expected %d components, got %d", n, len(all))
	}
}

func TestConcurrentRegistrationDuplicateRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	const goroutines = 10
	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := &mockComponent{name: "singleton", health: Health{Name: "singleton", Status: StatusHealthy}}
			if err := r.Register(c); err == nil {
				successCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if successCount.Load() != 1 {
		t.Errorf("expected exactly 1 successful registration, got %d", successCount.Load())
	}
}

// ---------------------------------------------------------------------------
// GAP 2: Context timeout/cancellation during StartAll/StopAll
// ---------------------------------------------------------------------------

func TestStartAllContextCancelled(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&slowComponent{name: "slow", delay: 5 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := r.StartAll(ctx)
	if err == nil {
		t.Fatal("expected error from StartAll with canceled context")
	}
}

func TestStopAllContextCancelled(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&slowComponent{name: "slow", delay: 5 * time.Second})

	// Start with a valid context
	startCtx, startCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer startCancel()
	r.StartAll(startCtx)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := r.StopAll(ctx)
	if err == nil {
		t.Fatal("expected error from StopAll with canceled context")
	}
}

// ---------------------------------------------------------------------------
// GAP 3: Race conditions — concurrent Stop + Health checks
// ---------------------------------------------------------------------------

func TestConcurrentStopAndHealth(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	for i := 0; i < 10; i++ {
		r.Register(&mockComponent{
			name:   fmt.Sprintf("c-%d", i),
			health: Health{Name: fmt.Sprintf("c-%d", i), Status: StatusHealthy},
		})
	}
	r.StartAll(context.Background())

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		r.StopAll(context.Background())
	}()
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			r.HealthAll(context.Background())
		}
	}()

	wg.Wait()
	// No panic or race detector complaint = pass
}

// ---------------------------------------------------------------------------
// GAP 4: StopAll error aggregation format
// ---------------------------------------------------------------------------

func TestStopAllErrorAggregationFormat(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&mockComponent{
		name: "db", stopErr: fmt.Errorf("db timeout"),
		health: Health{Name: "db", Status: StatusHealthy},
	})
	r.Register(&mockComponent{
		name: "cache", stopErr: fmt.Errorf("cache refused"),
		health: Health{Name: "cache", Status: StatusHealthy},
	})
	r.StartAll(context.Background())

	err := r.StopAll(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "shutdown errors") {
		t.Errorf("expected 'shutdown errors' prefix, got: %s", msg)
	}
	if !strings.Contains(msg, "db timeout") {
		t.Errorf("expected 'db timeout' in error, got: %s", msg)
	}
	if !strings.Contains(msg, "cache refused") {
		t.Errorf("expected 'cache refused' in error, got: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// GAP 5: LazyComponent concurrent Initialize
// ---------------------------------------------------------------------------

func TestLazyComponentConcurrentInitialize(t *testing.T) {
	t.Parallel()
	var callCount atomic.Int32
	lc := NewBaseLazyComponent("lazy-concurrent", func(ctx context.Context) error {
		callCount.Add(1)
		time.Sleep(10 * time.Millisecond) // simulate work
		return nil
	})

	var wg sync.WaitGroup
	const goroutines = 20
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lc.Initialize(context.Background())
		}()
	}
	wg.Wait()

	if callCount.Load() != 1 {
		t.Errorf("expected initializer called exactly once, got %d", callCount.Load())
	}
	if !lc.IsInitialized() {
		t.Error("expected component to be initialized")
	}
}

// ---------------------------------------------------------------------------
// GAP 6: LazyComponent init failure — state check
// ---------------------------------------------------------------------------

func TestLazyComponentInitFailureState(t *testing.T) {
	t.Parallel()
	lc := NewBaseLazyComponent("fail-init", func(ctx context.Context) error {
		return fmt.Errorf("init boom")
	})

	err := lc.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "init boom") {
		t.Errorf("expected 'init boom' in error, got: %v", err)
	}
	if lc.IsInitialized() {
		t.Error("expected not initialized after failure")
	}

	// HealthCheck should also fail
	if err := lc.HealthCheck(context.Background()); err == nil {
		t.Error("expected health check to fail for failed-init component")
	}
}

func TestLazyComponentInitFailureThenRetry(t *testing.T) {
	t.Parallel()
	callCount := 0
	lc := NewBaseLazyComponent("retry-init", func(ctx context.Context) error {
		callCount++
		if callCount == 1 {
			return fmt.Errorf("first try fails")
		}
		return nil
	})

	// First attempt fails
	err := lc.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected first init to fail")
	}

	// Second attempt succeeds (since initialized=false and lastError!=nil)
	err = lc.Initialize(context.Background())
	if err != nil {
		t.Fatalf("expected second init to succeed, got: %v", err)
	}
	if !lc.IsInitialized() {
		t.Error("expected initialized after successful retry")
	}
}

// ---------------------------------------------------------------------------
// GAP 7: LazyComponent Close with error
// ---------------------------------------------------------------------------

func TestLazyComponentCloseWithError(t *testing.T) {
	t.Parallel()
	lc := NewBaseLazyComponent("close-err", func(ctx context.Context) error { return nil })
	lc.WithCloser(func() error {
		return fmt.Errorf("close failed")
	})

	lc.Initialize(context.Background())
	err := lc.Close()
	if err == nil {
		t.Fatal("expected error from Close")
	}
	if !strings.Contains(err.Error(), "close failed") {
		t.Errorf("expected 'close failed', got: %v", err)
	}
	// Even on error, should be marked uninitialized
	if lc.IsInitialized() {
		t.Error("expected not initialized after Close even with error")
	}
}

func TestLazyComponentCloseWithoutInit(t *testing.T) {
	t.Parallel()
	closeCalled := false
	lc := NewBaseLazyComponent("not-init", func(ctx context.Context) error { return nil })
	lc.WithCloser(func() error {
		closeCalled = true
		return nil
	})

	if err := lc.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closeCalled {
		t.Error("closer should not be called when not initialized")
	}
}

// ---------------------------------------------------------------------------
// GAP 8: LazyComponent WithHealthCheck returning error
// ---------------------------------------------------------------------------

func TestLazyComponentHealthCheckCustomError(t *testing.T) {
	t.Parallel()
	lc := NewBaseLazyComponent("hc-err", func(ctx context.Context) error { return nil })
	lc.WithHealthCheck(func(ctx context.Context) error {
		return fmt.Errorf("degraded: high latency")
	})

	lc.Initialize(context.Background())
	err := lc.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected health check error")
	}
	if !strings.Contains(err.Error(), "degraded: high latency") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLazyComponentHealthCheckPassesAfterInit(t *testing.T) {
	t.Parallel()
	lc := NewBaseLazyComponent("hc-ok", func(ctx context.Context) error { return nil })
	lc.WithHealthCheck(func(ctx context.Context) error { return nil })

	lc.Initialize(context.Background())
	if err := lc.HealthCheck(context.Background()); err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GAP 9: Registry All() returns copy
// ---------------------------------------------------------------------------

func TestRegistryAllReturnsCopy(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&mockComponent{name: "a"})
	r.Register(&mockComponent{name: "b"})

	list1 := r.All()
	list2 := r.All()

	if len(list1) != 2 || len(list2) != 2 {
		t.Fatalf("expected 2 components each, got %d and %d", len(list1), len(list2))
	}

	// Modify one list, other should be unaffected
	list1[0] = nil
	if list2[0] == nil {
		t.Error("All() should return independent copies")
	}
}

// ---------------------------------------------------------------------------
// GAP 10: Context cancellation mid-lifecycle
// ---------------------------------------------------------------------------

func TestContextCancelMidStartAll(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	order := []string{}

	r.Register(&mockComponent{name: "a", startOrder: &order, health: Health{Name: "a", Status: StatusHealthy}})
	r.Register(&mockComponent{name: "b", startOrder: &order, health: Health{Name: "b", Status: StatusHealthy}})
	// 3rd component is slow — will see context cancel
	r.Register(&slowComponent{name: "c-slow", delay: 5 * time.Second, startOrder: &order})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := r.StartAll(ctx)
	if err == nil {
		t.Fatal("expected error when context expires mid-start")
	}
	// "a" and "b" should have started
	if len(order) < 2 {
		t.Errorf("expected at least 2 components started before cancel, got %v", order)
	}
}

// ---------------------------------------------------------------------------
// GAP 11: Empty name registration
// ---------------------------------------------------------------------------

func TestEmptyNameRegistration(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	err := r.Register(&mockComponent{name: ""})
	// Current impl allows empty name — verify it's retrievable
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := r.Get("")
	if got == nil {
		t.Error("expected to retrieve component with empty name")
	}
}

func TestEmptyNameDuplicate(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&mockComponent{name: ""})
	err := r.Register(&mockComponent{name: ""})
	if err == nil {
		t.Error("expected error for duplicate empty name")
	}
}

// ---------------------------------------------------------------------------
// GAP 12: Component Name with special chars / unicode
// ---------------------------------------------------------------------------

func TestComponentNameSpecialChars(t *testing.T) {
	t.Parallel()
	names := []string{
		"café-résumé",
		"数据库",
		"компонент",
		"name with spaces",
		"name/with/slashes",
		"name.with.dots",
		"<script>alert('xss')</script>",
	}
	r := NewRegistry()
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			c := &mockComponent{name: name, health: Health{Name: name, Status: StatusHealthy}}
			if err := r.Register(c); err != nil {
				t.Fatalf("failed to register %q: %v", name, err)
			}
			got := r.Get(name)
			if got == nil || got.Name() != name {
				t.Errorf("expected to get component %q back", name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GAP 13: Describable and RouteProvider interface satisfaction
// ---------------------------------------------------------------------------

func TestDescribableInterface(t *testing.T) {
	t.Parallel()
	dc := &describableComponent{
		mockComponent: mockComponent{name: "http-server"},
		desc: Description{
			Name:    "HTTP Server",
			Type:    "server",
			Details: "localhost:8080",
			Port:    8080,
		},
	}

	var _ Component = dc
	var _ Describable = dc

	desc := dc.Describe()
	if desc.Name != "HTTP Server" {
		t.Errorf("expected 'HTTP Server', got %q", desc.Name)
	}
	if desc.Type != "server" {
		t.Errorf("expected 'server', got %q", desc.Type)
	}
	if desc.Port != 8080 {
		t.Errorf("expected port 8080, got %d", desc.Port)
	}
}

func TestRouteProviderInterface(t *testing.T) {
	t.Parallel()
	rpc := &routeProviderComponent{
		mockComponent: mockComponent{name: "api"},
		routes: []Route{
			{Method: http.MethodGet, Path: "/health", Handler: "HealthHandler"},
			{Method: http.MethodPost, Path: "/users", Handler: "CreateUser"},
		},
	}

	var _ Component = rpc
	var _ RouteProvider = rpc

	routes := rpc.Routes()
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
	if routes[0].Method != "GET" || routes[0].Path != "/health" {
		t.Errorf("unexpected first route: %+v", routes[0])
	}
}

// ---------------------------------------------------------------------------
// GAP 14: Health with all three status values
// ---------------------------------------------------------------------------

func TestHealthAllThreeStatuses(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&mockComponent{name: "a", health: Health{Name: "a", Status: StatusHealthy, Message: "ok"}})
	r.Register(&mockComponent{name: "b", health: Health{Name: "b", Status: StatusDegraded, Message: "slow"}})
	r.Register(&mockComponent{name: "c", health: Health{Name: "c", Status: StatusUnhealthy, Message: "down"}})

	results := r.HealthAll(context.Background())
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	expected := []struct {
		status  HealthStatus
		message string
	}{
		{StatusHealthy, "ok"},
		{StatusDegraded, "slow"},
		{StatusUnhealthy, "down"},
	}
	for i, e := range expected {
		if results[i].Status != e.status {
			t.Errorf("[%d] expected status %s, got %s", i, e.status, results[i].Status)
		}
		if results[i].Message != e.message {
			t.Errorf("[%d] expected message %q, got %q", i, e.message, results[i].Message)
		}
	}
}

// ---------------------------------------------------------------------------
// GAP 15: Large number of components (50+)
// ---------------------------------------------------------------------------

func TestLargeComponentCount(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	const n = 100
	startOrder := []string{}
	stopOrder := []string{}

	for i := 0; i < n; i++ {
		name := fmt.Sprintf("comp-%03d", i)
		r.Register(&mockComponent{
			name:       name,
			startOrder: &startOrder,
			stopOrder:  &stopOrder,
			health:     Health{Name: name, Status: StatusHealthy},
		})
	}

	if err := r.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}
	if len(startOrder) != n {
		t.Fatalf("expected %d starts, got %d", n, len(startOrder))
	}
	// Verify registration order preserved
	for i := 0; i < n; i++ {
		expected := fmt.Sprintf("comp-%03d", i)
		if startOrder[i] != expected {
			t.Errorf("start order[%d]: expected %s, got %s", i, expected, startOrder[i])
			break
		}
	}

	if err := r.StopAll(context.Background()); err != nil {
		t.Fatalf("StopAll failed: %v", err)
	}
	// Verify reverse order
	for i := 0; i < n; i++ {
		expected := fmt.Sprintf("comp-%03d", n-1-i)
		if stopOrder[i] != expected {
			t.Errorf("stop order[%d]: expected %s, got %s", i, expected, stopOrder[i])
			break
		}
	}

	healths := r.HealthAll(context.Background())
	if len(healths) != n {
		t.Errorf("expected %d health results, got %d", n, len(healths))
	}
}

// ---------------------------------------------------------------------------
// GAP 16: Double StartAll / StopAll (idempotency)
// ---------------------------------------------------------------------------

func TestDoubleStartAll(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	counter := &countingComponent{name: "counter", health: Health{Name: "counter", Status: StatusHealthy}}
	r.Register(counter)

	if err := r.StartAll(context.Background()); err != nil {
		t.Fatalf("first StartAll failed: %v", err)
	}
	if err := r.StartAll(context.Background()); err != nil {
		t.Fatalf("second StartAll failed: %v", err)
	}
	// Idempotent: Start should be called only once since the component is already running.
	if counter.startCount != 1 {
		t.Errorf("expected Start called 1 time (idempotent), got %d", counter.startCount)
	}
}

func TestDoubleStopAll(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	counter := &countingComponent{name: "counter", health: Health{Name: "counter", Status: StatusHealthy}}
	r.Register(counter)

	r.StartAll(context.Background())
	r.StopAll(context.Background())
	// Second StopAll should be a no-op (component already marked not started)
	r.StopAll(context.Background())

	if counter.stopCount != 1 {
		t.Errorf("expected Stop called 1 time, got %d", counter.stopCount)
	}
}

// ---------------------------------------------------------------------------
// Two-phase startup: register after StartAll, then StartAll again
// ---------------------------------------------------------------------------

func TestStartAllTwoPhase(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	order := []string{}

	// Phase 1: infrastructure
	infra := &mockComponent{name: "db", startOrder: &order, health: Health{Name: "db", Status: StatusHealthy}}
	r.Register(infra)

	if err := r.StartAll(context.Background()); err != nil {
		t.Fatalf("Phase 1 StartAll failed: %v", err)
	}
	if len(order) != 1 || order[0] != "db" {
		t.Fatalf("expected [db], got %v", order)
	}

	// Phase 2: application components registered after first StartAll
	app1 := &mockComponent{name: "worker", startOrder: &order, health: Health{Name: "worker", Status: StatusHealthy}}
	app2 := &mockComponent{name: "scheduler", startOrder: &order, health: Health{Name: "scheduler", Status: StatusHealthy}}
	r.Register(app1)
	r.Register(app2)

	if err := r.StartAll(context.Background()); err != nil {
		t.Fatalf("Phase 2 StartAll failed: %v", err)
	}

	// db should NOT be started again; worker and scheduler should be started in order
	if len(order) != 3 {
		t.Fatalf("expected 3 total starts, got %d: %v", len(order), order)
	}
	if order[1] != "worker" || order[2] != "scheduler" {
		t.Errorf("expected [db, worker, scheduler], got %v", order)
	}
}

func TestStartAllTwoPhaseRollback(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	order := []string{}
	stopOrder := []string{}

	// Phase 1
	r.Register(&mockComponent{name: "db", startOrder: &order, stopOrder: &stopOrder, health: Health{Name: "db", Status: StatusHealthy}})
	r.StartAll(context.Background())

	// Phase 2: second component fails
	r.Register(&mockComponent{name: "worker-ok", startOrder: &order, stopOrder: &stopOrder, health: Health{Name: "worker-ok", Status: StatusHealthy}})
	r.Register(&mockComponent{name: "worker-fail", startOrder: &order, stopOrder: &stopOrder, startErr: fmt.Errorf("boom"), health: Health{Name: "worker-fail", Status: StatusHealthy}})
	r.Register(&mockComponent{name: "worker-never", startOrder: &order, stopOrder: &stopOrder, health: Health{Name: "worker-never", Status: StatusHealthy}})

	err := r.StartAll(context.Background())
	if err == nil {
		t.Fatal("expected error from second StartAll")
	}

	// worker-ok was started then rolled back; worker-fail failed; worker-never never started
	if len(order) != 3 { // db, worker-ok, worker-fail (attempted)
		t.Fatalf("expected 3 start attempts, got %d: %v", len(order), order)
	}
	// Rollback should stop only worker-ok (the one started in this call)
	if len(stopOrder) != 1 || stopOrder[0] != "worker-ok" {
		t.Errorf("expected rollback of [worker-ok], got %v", stopOrder)
	}

	// db should still be started (from phase 1, not rolled back)
	all := r.All()
	for _, c := range all {
		if c.Name() == "db" {
			h := c.Health(context.Background())
			if h.Status != StatusHealthy {
				t.Errorf("db should still be healthy after phase 2 rollback")
			}
		}
	}
}

func TestStartAllTwoPhaseStopAll(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	stopOrder := []string{}

	// Phase 1: infra
	r.Register(&mockComponent{name: "db", stopOrder: &stopOrder, health: Health{Name: "db", Status: StatusHealthy}})
	r.StartAll(context.Background())

	// Phase 2: app
	r.Register(&mockComponent{name: "worker", stopOrder: &stopOrder, health: Health{Name: "worker", Status: StatusHealthy}})
	r.StartAll(context.Background())

	// StopAll should stop both in reverse order
	r.StopAll(context.Background())
	if len(stopOrder) != 2 {
		t.Fatalf("expected 2 stops, got %d: %v", len(stopOrder), stopOrder)
	}
	if stopOrder[0] != "worker" || stopOrder[1] != "db" {
		t.Errorf("expected reverse stop [worker, db], got %v", stopOrder)
	}
}

// countingComponent tracks how many times Start/Stop are called.
type countingComponent struct {
	name       string
	health     Health
	startCount int
	stopCount  int
}

func (c *countingComponent) Name() string                    { return c.name }
func (c *countingComponent) Start(_ context.Context) error   { c.startCount++; return nil }
func (c *countingComponent) Stop(_ context.Context) error    { c.stopCount++; return nil }
func (c *countingComponent) Health(_ context.Context) Health { return c.health }

// ---------------------------------------------------------------------------
// GAP 17: Security — injection in component names / health messages
// ---------------------------------------------------------------------------

func TestSecurityInjectionInNames(t *testing.T) {
	t.Parallel()
	injectionNames := []string{
		"'; DROP TABLE components; --",
		"<img src=x onerror=alert(1)>",
		"${ENV_VAR}",
		"../../../etc/passwd",
		"name\x00null",
	}
	r := NewRegistry()
	for _, name := range injectionNames {
		c := &mockComponent{name: name, health: Health{Name: name, Status: StatusHealthy}}
		if err := r.Register(c); err != nil {
			t.Fatalf("register %q failed: %v", name, err)
		}
		got := r.Get(name)
		if got == nil {
			t.Errorf("expected to retrieve component with name %q", name)
		}
		if got != nil && got.Name() != name {
			t.Errorf("name mismatch: expected %q, got %q", name, got.Name())
		}
	}
}

func TestSecurityInjectionInHealthMessages(t *testing.T) {
	t.Parallel()
	msg := "<script>alert('xss')</script>"
	r := NewRegistry()
	r.Register(&mockComponent{
		name:   "sec",
		health: Health{Name: "sec", Status: StatusDegraded, Message: msg},
	})

	results := r.HealthAll(context.Background())
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}
	if results[0].Message != msg {
		t.Errorf("expected message preserved as-is, got %q", results[0].Message)
	}
}

// ---------------------------------------------------------------------------
// GAP: StartAll partial failure
// ---------------------------------------------------------------------------

func TestStartAllPartialFailure(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	order := []string{}
	r.Register(&mockComponent{name: "a", startOrder: &order, health: Health{Name: "a", Status: StatusHealthy}})
	r.Register(&mockComponent{name: "b", startOrder: &order, startErr: fmt.Errorf("b exploded"), health: Health{Name: "b", Status: StatusHealthy}})
	r.Register(&mockComponent{name: "c", startOrder: &order, health: Health{Name: "c", Status: StatusHealthy}})

	err := r.StartAll(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	// "a" should have started, "b" failed, "c" never attempted
	if len(order) != 2 {
		t.Errorf("expected [a, b] in order, got %v", order)
	}
	if order[0] != "a" || order[1] != "b" {
		t.Errorf("expected [a, b], got %v", order)
	}
}

// ---------------------------------------------------------------------------
// GAP: LazyComponent re-init after Close
// ---------------------------------------------------------------------------

func TestLazyComponentReInitAfterClose(t *testing.T) {
	t.Parallel()
	initCount := 0
	lc := NewBaseLazyComponent("reinit", func(ctx context.Context) error {
		initCount++
		return nil
	})

	lc.Initialize(context.Background())
	if !lc.IsInitialized() {
		t.Fatal("expected initialized")
	}

	lc.Close()
	if lc.IsInitialized() {
		t.Fatal("expected not initialized after close")
	}

	lc.Initialize(context.Background())
	if !lc.IsInitialized() {
		t.Fatal("expected re-initialized")
	}
	if initCount != 2 {
		t.Errorf("expected 2 init calls, got %d", initCount)
	}
}

// ---------------------------------------------------------------------------
// GAP: LazyComponent builder chaining
// ---------------------------------------------------------------------------

func TestLazyComponentBuilderChaining(t *testing.T) {
	t.Parallel()
	lc := NewBaseLazyComponent("chain", func(ctx context.Context) error { return nil }).
		WithHealthCheck(func(ctx context.Context) error { return nil }).
		WithCloser(func() error { return nil })

	if lc.Name() != "chain" {
		t.Errorf("expected 'chain', got %q", lc.Name())
	}
	lc.Initialize(context.Background())
	if err := lc.HealthCheck(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := lc.Close(); err != nil {
		t.Errorf("unexpected close error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GAP: Description struct zero values
// ---------------------------------------------------------------------------

func TestDescriptionZeroValues(t *testing.T) {
	t.Parallel()
	d := Description{}
	if d.Name != "" || d.Type != "" || d.Details != "" || d.Port != 0 {
		t.Errorf("expected zero Description, got %+v", d)
	}
}

// ---------------------------------------------------------------------------
// GAP: Health struct zero values and all fields
// ---------------------------------------------------------------------------

func TestHealthStructFields(t *testing.T) {
	t.Parallel()
	h := Health{Name: "test", Status: StatusDegraded, Message: "partial"}
	if h.Name != "test" {
		t.Errorf("expected name 'test', got %q", h.Name)
	}
	if h.Status != StatusDegraded {
		t.Errorf("expected 'degraded', got %q", h.Status)
	}
	if h.Message != "partial" {
		t.Errorf("expected 'partial', got %q", h.Message)
	}
}

// ---------------------------------------------------------------------------
// GAP: Route struct
// ---------------------------------------------------------------------------

func TestRouteStruct(t *testing.T) {
	t.Parallel()
	r := Route{Method: "POST", Path: "/api/v1/users", Handler: "CreateUser"}
	if r.Method != "POST" || r.Path != "/api/v1/users" || r.Handler != "CreateUser" {
		t.Errorf("unexpected route: %+v", r)
	}
}
