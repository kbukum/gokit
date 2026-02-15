package bootstrap

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/skillsenselab/gokit/component"
	"github.com/skillsenselab/gokit/di"
)

// mockComponent implements component.Component for testing.
type mockComponent struct {
	name       string
	startErr   error
	stopErr    error
	health     component.ComponentHealth
	started    bool
	stopped    bool
}

func (m *mockComponent) Name() string { return m.name }
func (m *mockComponent) Start(ctx context.Context) error {
	m.started = true
	return m.startErr
}
func (m *mockComponent) Stop(ctx context.Context) error {
	m.stopped = true
	return m.stopErr
}
func (m *mockComponent) Health(ctx context.Context) component.ComponentHealth {
	return m.health
}

func TestNewApp(t *testing.T) {
	app := NewApp("test-svc", "1.0.0")
	if app == nil {
		t.Fatal("expected non-nil app")
	}
	if app.Name != "test-svc" {
		t.Errorf("expected name 'test-svc', got %q", app.Name)
	}
	if app.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", app.Version)
	}
	if app.Container == nil {
		t.Error("expected non-nil container")
	}
	if app.Components == nil {
		t.Error("expected non-nil components registry")
	}
	if app.Logger == nil {
		t.Error("expected non-nil logger")
	}
}

func TestNewAppWithOptions(t *testing.T) {
	container := di.NewContainer()
	app := NewApp("test", "1.0",
		WithGracefulTimeout(30*time.Second),
		WithContainer(container),
	)

	if app.gracefulTimeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", app.gracefulTimeout)
	}
	if app.Container != container {
		t.Error("expected custom container")
	}
}

func TestRegisterComponent(t *testing.T) {
	app := NewApp("test", "1.0")
	c := &mockComponent{
		name:   "db",
		health: component.ComponentHealth{Name: "db", Status: component.StatusHealthy},
	}

	if err := app.RegisterComponent(c); err != nil {
		t.Fatalf("RegisterComponent failed: %v", err)
	}

	got := app.Components.Get("db")
	if got == nil {
		t.Error("expected component to be registered")
	}
}

func TestRegisterComponentDuplicate(t *testing.T) {
	app := NewApp("test", "1.0")
	c := &mockComponent{name: "db"}
	app.RegisterComponent(c)

	err := app.RegisterComponent(&mockComponent{name: "db"})
	if err == nil {
		t.Error("expected error for duplicate component registration")
	}
}

func TestOnStartHook(t *testing.T) {
	app := NewApp("test", "1.0")
	called := false
	app.OnStart(func(ctx context.Context) error {
		called = true
		return nil
	})

	if len(app.onStart) != 1 {
		t.Errorf("expected 1 onStart hook, got %d", len(app.onStart))
	}

	// Simulate running hooks
	err := runHooks(context.Background(), app.onStart)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}
	if !called {
		t.Error("expected onStart hook to be called")
	}
}

func TestOnReadyHook(t *testing.T) {
	app := NewApp("test", "1.0")
	called := false
	app.OnReady(func(ctx context.Context) error {
		called = true
		return nil
	})

	err := runHooks(context.Background(), app.onReady)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}
	if !called {
		t.Error("expected onReady hook to be called")
	}
}

func TestOnStopHook(t *testing.T) {
	app := NewApp("test", "1.0")
	called := false
	app.OnStop(func(ctx context.Context) error {
		called = true
		return nil
	})

	err := runHooks(context.Background(), app.onStop)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}
	if !called {
		t.Error("expected onStop hook to be called")
	}
}

func TestMultipleHooks(t *testing.T) {
	app := NewApp("test", "1.0")
	order := []string{}
	app.OnStart(
		func(ctx context.Context) error { order = append(order, "first"); return nil },
		func(ctx context.Context) error { order = append(order, "second"); return nil },
	)

	runHooks(context.Background(), app.onStart)
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("expected [first, second], got %v", order)
	}
}

func TestHookError(t *testing.T) {
	hooks := []Hook{
		func(ctx context.Context) error { return fmt.Errorf("hook failed") },
	}
	err := runHooks(context.Background(), hooks)
	if err == nil {
		t.Error("expected error from failing hook")
	}
}

func TestHookErrorStopsExecution(t *testing.T) {
	secondCalled := false
	hooks := []Hook{
		func(ctx context.Context) error { return fmt.Errorf("fail") },
		func(ctx context.Context) error { secondCalled = true; return nil },
	}
	runHooks(context.Background(), hooks)
	if secondCalled {
		t.Error("expected second hook not to be called after first fails")
	}
}

func TestReadyCheckAllHealthy(t *testing.T) {
	app := NewApp("test", "1.0")
	app.RegisterComponent(&mockComponent{
		name:   "db",
		health: component.ComponentHealth{Name: "db", Status: component.StatusHealthy},
	})
	app.RegisterComponent(&mockComponent{
		name:   "cache",
		health: component.ComponentHealth{Name: "cache", Status: component.StatusHealthy},
	})

	err := app.ReadyCheck(context.Background())
	if err != nil {
		t.Errorf("expected no error for all healthy, got %v", err)
	}
}

func TestReadyCheckUnhealthy(t *testing.T) {
	app := NewApp("test", "1.0")
	app.RegisterComponent(&mockComponent{
		name:   "db",
		health: component.ComponentHealth{Name: "db", Status: component.StatusHealthy},
	})
	app.RegisterComponent(&mockComponent{
		name:   "cache",
		health: component.ComponentHealth{Name: "cache", Status: component.StatusUnhealthy, Message: "timeout"},
	})

	err := app.ReadyCheck(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy component")
	}
}

func TestReadyCheckDegraded(t *testing.T) {
	app := NewApp("test", "1.0")
	app.RegisterComponent(&mockComponent{
		name:   "svc",
		health: component.ComponentHealth{Name: "svc", Status: component.StatusDegraded, Message: "slow"},
	})

	err := app.ReadyCheck(context.Background())
	if err == nil {
		t.Error("expected error for degraded component")
	}
}

func TestReadyCheckEmpty(t *testing.T) {
	app := NewApp("test", "1.0")
	err := app.ReadyCheck(context.Background())
	if err != nil {
		t.Errorf("expected no error for empty registry, got %v", err)
	}
}

func TestOnConfigure(t *testing.T) {
	app := NewApp("test", "1.0")
	configured := false
	app.OnConfigure(func(ctx context.Context, a *App) error {
		configured = true
		if a.Name != "test" {
			t.Errorf("expected app name 'test' in configure callback, got %q", a.Name)
		}
		return nil
	})

	if len(app.onConfigure) != 1 {
		t.Errorf("expected 1 configure callback, got %d", len(app.onConfigure))
	}

	// Simulate configure phase
	for _, fn := range app.onConfigure {
		if err := fn(context.Background(), app); err != nil {
			t.Fatalf("configure failed: %v", err)
		}
	}
	if !configured {
		t.Error("expected configure callback to run")
	}
}

func TestWithGracefulTimeout(t *testing.T) {
	app := NewApp("test", "1.0", WithGracefulTimeout(5*time.Second))
	if app.gracefulTimeout != 5*time.Second {
		t.Errorf("expected 5s, got %v", app.gracefulTimeout)
	}
}

func TestDefaultGracefulTimeout(t *testing.T) {
	app := NewApp("test", "1.0")
	if app.gracefulTimeout != 15*time.Second {
		t.Errorf("expected default 15s, got %v", app.gracefulTimeout)
	}
}
