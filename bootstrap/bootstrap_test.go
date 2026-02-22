package bootstrap

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/config"
	"github.com/kbukum/gokit/di"
)

// testConfig is a minimal config for testing that satisfies the Config interface.
type testConfig struct {
	config.ServiceConfig
}

// mockComponent implements component.Component for testing.
type mockComponent struct {
	name     string
	startErr error
	stopErr  error
	health   component.Health
	started  bool
	stopped  bool
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
func (m *mockComponent) Health(ctx context.Context) component.Health {
	return m.health
}

func newTestConfig(name, version string) *testConfig {
	return &testConfig{
		ServiceConfig: config.ServiceConfig{
			Name:        name,
			Version:     version,
			Environment: "development",
		},
	}
}

func TestNewApp(t *testing.T) {
	cfg := newTestConfig("test-svc", "1.0.0")
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
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
	// Config is typed
	if app.Cfg.Name != "test-svc" {
		t.Errorf("expected cfg.Name 'test-svc', got %q", app.Cfg.Name)
	}
}

func TestNewAppValidation(t *testing.T) {
	cfg := &testConfig{
		ServiceConfig: config.ServiceConfig{
			// Name is empty — should fail validation
			Environment: "development",
		},
	}
	_, err := NewApp(cfg)
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestNewAppWithOptions(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	container := di.NewContainer()
	app, err := NewApp(cfg,
		WithGracefulTimeout(30*time.Second),
		WithContainer(container),
	)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}

	if app.gracefulTimeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", app.gracefulTimeout)
	}
	if app.Container != container {
		t.Error("expected custom container")
	}
}

func TestRegisterComponent(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	c := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
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
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	c := &mockComponent{name: "db"}
	app.RegisterComponent(c)

	err := app.RegisterComponent(&mockComponent{name: "db"})
	if err == nil {
		t.Error("expected error for duplicate component registration")
	}
}

func TestOnStartHook(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	called := false
	app.OnStart(func(ctx context.Context) error {
		called = true
		return nil
	})

	if len(app.onStart) != 1 {
		t.Errorf("expected 1 onStart hook, got %d", len(app.onStart))
	}

	err := runHooks(context.Background(), app.onStart)
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}
	if !called {
		t.Error("expected onStart hook to be called")
	}
}

func TestOnReadyHook(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
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
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
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
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
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
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.RegisterComponent(&mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	})
	app.RegisterComponent(&mockComponent{
		name:   "cache",
		health: component.Health{Name: "cache", Status: component.StatusHealthy},
	})

	err := app.ReadyCheck(context.Background())
	if err != nil {
		t.Errorf("expected no error for all healthy, got %v", err)
	}
}

func TestReadyCheckUnhealthy(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.RegisterComponent(&mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	})
	app.RegisterComponent(&mockComponent{
		name:   "cache",
		health: component.Health{Name: "cache", Status: component.StatusUnhealthy, Message: "timeout"},
	})

	err := app.ReadyCheck(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy component")
	}
}

func TestReadyCheckDegraded(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.RegisterComponent(&mockComponent{
		name:   "svc",
		health: component.Health{Name: "svc", Status: component.StatusDegraded, Message: "slow"},
	})

	err := app.ReadyCheck(context.Background())
	if err == nil {
		t.Error("expected error for degraded component")
	}
}

func TestReadyCheckEmpty(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	err := app.ReadyCheck(context.Background())
	if err != nil {
		t.Errorf("expected no error for empty registry, got %v", err)
	}
}

func TestOnConfigure(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	configured := false
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		configured = true
		if a.Name != "test" {
			t.Errorf("expected app name 'test' in configure callback, got %q", a.Name)
		}
		// Type-safe config access
		if a.Cfg.Name != "test" {
			t.Errorf("expected cfg.Name 'test', got %q", a.Cfg.Name)
		}
		return nil
	})

	if len(app.onConfigure) != 1 {
		t.Errorf("expected 1 configure callback, got %d", len(app.onConfigure))
	}

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
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg, WithGracefulTimeout(5*time.Second))
	if app.gracefulTimeout != 5*time.Second {
		t.Errorf("expected 5s, got %v", app.gracefulTimeout)
	}
}

func TestDefaultGracefulTimeout(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	if app.gracefulTimeout != 15*time.Second {
		t.Errorf("expected default 15s, got %v", app.gracefulTimeout)
	}
}

func TestRunTaskSuccess(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	executed := false
	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if !executed {
		t.Error("expected task to be executed")
	}
}

func TestRunTaskError(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return fmt.Errorf("task error")
	})
	if err == nil {
		t.Error("expected error from failing task")
	}
	if err.Error() != "task error" {
		t.Errorf("expected 'task error', got %q", err.Error())
	}
}

func TestRunTaskCancellation(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	ctx, cancel := context.WithCancel(context.Background())

	err := app.RunTask(ctx, func(taskCtx context.Context) error {
		cancel() // simulate signal
		<-taskCtx.Done()
		return taskCtx.Err()
	})
	if err == nil {
		t.Error("expected error from cancelled task")
	}
}

func TestRunTaskWithHooks(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	order := []string{}
	app.OnStart(func(ctx context.Context) error {
		order = append(order, "start")
		return nil
	})
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		order = append(order, "configure")
		return nil
	})
	app.OnReady(func(ctx context.Context) error {
		order = append(order, "ready")
		return nil
	})
	app.OnStop(func(ctx context.Context) error {
		order = append(order, "stop")
		return nil
	})

	app.RunTask(context.Background(), func(ctx context.Context) error {
		order = append(order, "task")
		return nil
	})

	expected := []string{"start", "configure", "ready", "task", "stop"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, expected %q", i, order[i], v)
		}
	}
}

func TestRunTaskWithComponents(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	comp := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if !comp.started {
		t.Error("expected component to be started")
	}
	if !comp.stopped {
		t.Error("expected component to be stopped after task")
	}
}
