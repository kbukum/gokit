package component

import (
	"context"
	"fmt"
	"testing"
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
