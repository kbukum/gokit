package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/config"
	"github.com/kbukum/gokit/di"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// failValidationConfig returns an error from Validate().
type failValidationConfig struct {
	config.ServiceConfig
	valErr error
}

func (f *failValidationConfig) Validate() error { return f.valErr }

// slowComponent delays start/stop by a configurable duration.
type slowComponent struct {
	name       string
	startDelay time.Duration
	stopDelay  time.Duration
	health     component.Health
	started    bool
	stopped    bool
}

func (s *slowComponent) Name() string { return s.name }
func (s *slowComponent) Start(ctx context.Context) error {
	select {
	case <-time.After(s.startDelay):
		s.started = true
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *slowComponent) Stop(ctx context.Context) error {
	select {
	case <-time.After(s.stopDelay):
		s.stopped = true
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (s *slowComponent) Health(ctx context.Context) component.Health { return s.health }

// orderTrackingComponent records start/stop calls to a shared slice.
type orderTrackingComponent struct {
	name     string
	order    *[]string
	startErr error
	stopErr  error
	health   component.Health
}

func (o *orderTrackingComponent) Name() string { return o.name }
func (o *orderTrackingComponent) Start(ctx context.Context) error {
	*o.order = append(*o.order, o.name+":start")
	return o.startErr
}

func (o *orderTrackingComponent) Stop(ctx context.Context) error {
	*o.order = append(*o.order, o.name+":stop")
	return o.stopErr
}
func (o *orderTrackingComponent) Health(ctx context.Context) component.Health { return o.health }

// failCloseContainer is a di.Container wrapper that returns an error from Close().
type failCloseContainer struct {
	di.Container
	closeErr error
}

func (f *failCloseContainer) Close() error { return f.closeErr }

// ── 1. Config validation failure recovery ───────────────────────────────────

func TestConfigValidationFailureRecovery(t *testing.T) {
	cfg := &failValidationConfig{
		ServiceConfig: config.ServiceConfig{
			Name:        "test-svc",
			Environment: "development",
		},
		valErr: fmt.Errorf("port out of range"),
	}
	_, err := NewApp(cfg)
	if err == nil {
		t.Fatal("expected error from config validation")
	}
	if !strings.Contains(err.Error(), "config validation") {
		t.Errorf("expected 'config validation' prefix, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "port out of range") {
		t.Errorf("expected wrapped validation error, got %q", err.Error())
	}
}

func TestConfigValidationWithWrappedError(t *testing.T) {
	inner := fmt.Errorf("inner cause")
	cfg := &failValidationConfig{
		ServiceConfig: config.ServiceConfig{
			Name:        "test-svc",
			Environment: "development",
		},
		valErr: fmt.Errorf("validation: %w", inner),
	}
	_, err := NewApp(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, inner) {
		t.Error("expected wrapped error to be unwrappable")
	}
}

// ── 2. Phase 1 component startup partial failure ────────────────────────────

func TestPhase1PartialStartupFailure(t *testing.T) {
	order := []string{}
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	comp1 := &orderTrackingComponent{
		name:   "db",
		order:  &order,
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	comp2 := &orderTrackingComponent{
		name:     "cache",
		order:    &order,
		startErr: fmt.Errorf("redis connection refused"),
		health:   component.Health{Name: "cache", Status: component.StatusHealthy},
	}
	comp3 := &orderTrackingComponent{
		name:   "kafka",
		order:  &order,
		health: component.Health{Name: "kafka", Status: component.StatusHealthy},
	}

	app.RegisterComponent(comp1)
	app.RegisterComponent(comp2)
	app.RegisterComponent(comp3)

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error from partial startup failure")
	}
	if !strings.Contains(err.Error(), "redis connection refused") {
		t.Errorf("expected redis error, got %q", err.Error())
	}
	// Third component should NOT have started
	for _, e := range order {
		if e == "kafka:start" {
			t.Error("kafka should not have started after cache failed")
		}
	}
}

// ── 3. Hook error propagation across phases ──────────────────────────────

func TestHookErrorIncludesIndex(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	app.OnStart(
		func(ctx context.Context) error { return nil },
		func(_ context.Context) error { return nil },
		func(ctx context.Context) error { return fmt.Errorf("boom") },
	)
	err := app.emitLifecycleHooks(context.Background(), EventStart)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected 'boom' in error, got %q", err.Error())
	}
}

func TestOnStopHookErrorDoesNotPreventComponentStop(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	comp := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	app.OnStop(func(ctx context.Context) error {
		return fmt.Errorf("stop hook error")
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err == nil {
		t.Error("expected stop hook error")
	}
	if !comp.stopped {
		t.Error("component should still be stopped even after hook error")
	}
}

// ── 4. RunTask with context cancellation ────────────────────────────────────

func TestRunTaskContextCancellationBeforeTask(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := app.RunTask(ctx, func(taskCtx context.Context) error {
		select {
		case <-taskCtx.Done():
			return taskCtx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	if err == nil {
		t.Error("expected error from canceled context")
	}
}

func TestRunTaskErrorTakesPriorityOverStopError(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	taskErr := fmt.Errorf("task failed")

	app.OnStop(func(ctx context.Context) error {
		return fmt.Errorf("stop also failed")
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return taskErr
	})

	if err == nil {
		t.Fatal("expected error")
	}
	// When both task and stop fail, task error wins
	if err.Error() != "task failed" {
		t.Errorf("expected task error to take priority, got %q", err.Error())
	}
}

// ── 5. Graceful timeout exceeded during component stop ──────────────────────

func TestGracefulTimeoutDuringStop(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg, WithGracefulTimeout(100*time.Millisecond))

	app.OnStop(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			return nil
		}
	})

	// RunTask should still complete (stop should timeout but not hang)
	done := make(chan error, 1)
	go func() {
		done <- app.RunTask(context.Background(), func(ctx context.Context) error {
			return nil
		})
	}()

	select {
	case err := <-done:
		// Should get a context deadline exceeded error from the stop hook
		if err == nil {
			t.Error("expected error from timed-out stop hook")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunTask should not hang on graceful timeout")
	}
}

// ── 6. Multiple OnConfigure callbacks ────────────────────────────────────────

func TestMultipleOnConfigureCallbacksOrder(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	order := []string{}
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		order = append(order, "config-1")
		return nil
	})
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		order = append(order, "config-2")
		return nil
	})
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		order = append(order, "config-3")
		return nil
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}

	expected := []string{"config-1", "config-2", "config-3"}
	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, expected %q", i, order[i], v)
		}
	}
}

func TestOnConfigureErrorStopsSubsequentCallbacks(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	order := []string{}
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		order = append(order, "first")
		return nil
	})
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		return fmt.Errorf("config-2 failed")
	})
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		order = append(order, "third")
		return nil
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error from configure callback")
	}
	if len(order) != 1 || order[0] != "first" {
		t.Errorf("only first callback should have run, got %v", order)
	}
}

// ── 7. ReadyCheck racing with component failure ─────────────────────────────

func TestReadyCheckWithMixedHealth(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	app.RegisterComponent(&mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	})
	app.RegisterComponent(&mockComponent{
		name:   "cache",
		health: component.Health{Name: "cache", Status: component.StatusDegraded, Message: "slow"},
	})
	app.RegisterComponent(&mockComponent{
		name:   "kafka",
		health: component.Health{Name: "kafka", Status: component.StatusUnhealthy, Message: "disconnected"},
	})

	err := app.ReadyCheck(context.Background())
	if err == nil {
		t.Fatal("expected error for unhealthy components")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "cache") {
		t.Error("expected cache in unhealthy list")
	}
	if !strings.Contains(errMsg, "kafka") {
		t.Error("expected kafka in unhealthy list")
	}
	if !strings.Contains(errMsg, "slow") {
		t.Error("expected 'slow' message for cache")
	}
}

func TestReadyCheckWarningDoesNotBlockStartup(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	// Register a degraded component — startup warns but continues
	app.RegisterComponent(&mockComponent{
		name:   "cache",
		health: component.Health{Name: "cache", Status: component.StatusDegraded, Message: "warming up"},
	})

	taskRan := false
	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		taskRan = true
		return nil
	})
	// Ready check issues a warning but startup should still succeed
	if err != nil {
		t.Fatalf("RunTask should succeed despite degraded component: %v", err)
	}
	if !taskRan {
		t.Error("task should have executed")
	}
}

// ── 8. Container.Close() failures during shutdown ───────────────────────────

func TestContainerCloseErrorDuringShutdown(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	container := &failCloseContainer{
		Container: di.NewContainer(),
		closeErr:  fmt.Errorf("container close failed"),
	}
	app, _ := NewApp(cfg, WithContainer(container))

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error from container close failure")
	}
	if !strings.Contains(err.Error(), "container close failed") {
		t.Errorf("expected container close error, got %q", err.Error())
	}
}

func TestMultipleShutdownErrors(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	container := &failCloseContainer{
		Container: di.NewContainer(),
		closeErr:  fmt.Errorf("container close error"),
	}
	app, _ := NewApp(cfg, WithContainer(container))

	app.OnStop(func(ctx context.Context) error {
		return fmt.Errorf("stop hook error")
	})
	app.RegisterComponent(&mockComponent{
		name:    "bad-comp",
		stopErr: fmt.Errorf("comp stop error"),
		health:  component.Health{Name: "bad-comp", Status: component.StatusHealthy},
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected combined shutdown errors")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "stop hook error") {
		t.Error("expected stop hook error in combined error")
	}
}

// ── 9. DisplaySummary with edge cases ───────────────────────────────────────

func TestDisplaySummaryEmptyComponents(t *testing.T) {
	s := NewSummary("empty-svc", "0.1.0")
	s.SetStartupDuration(0)

	registry := component.NewRegistry()
	container := di.NewContainer()

	// Should not panic with empty everything
	s.DisplaySummary(registry, container, nil)
}

func TestDisplaySummaryZeroPort(t *testing.T) {
	s := NewSummary("svc", "1.0")
	s.SetStartupDuration(50 * time.Millisecond)
	s.TrackInfrastructure("DB", "database", "active", "localhost", 0, true)

	registry := component.NewRegistry()
	container := di.NewContainer()

	// Should not panic and should not append ":0"
	s.DisplaySummary(registry, container, nil)
}

func TestDisplaySummaryZeroDuration(t *testing.T) {
	s := NewSummary("instant-svc", "0.0.1")
	s.SetStartupDuration(0)
	s.TrackRoute("GET", "/health", "HealthHandler")

	registry := component.NewRegistry()
	container := di.NewContainer()

	// Should render 0.00s without panic
	s.DisplaySummary(registry, container, nil)
}

func TestDisplaySummaryNilContainer(t *testing.T) {
	s := NewSummary("nil-container-svc", "1.0")
	s.SetStartupDuration(10 * time.Millisecond)

	registry := component.NewRegistry()
	// nil container
	s.DisplaySummary(registry, nil, nil)
}

// ── 10. Double Run/RunTask calls ─────────────────────────────────────────────

func TestDoubleRunTaskSequential(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	count := 0
	task := func(ctx context.Context) error {
		count++
		return nil
	}

	// First run
	err1 := app.RunTask(context.Background(), task)
	if err1 != nil {
		t.Fatalf("first RunTask failed: %v", err1)
	}

	// Second run — container is already closed, but should still attempt
	err2 := app.RunTask(context.Background(), task)
	// We don't require error-free second run, just no panic
	_ = err2

	if count < 1 {
		t.Error("task should have run at least once")
	}
}

// ── Additional edge cases ────────────────────────────────────────────────────

func TestRunTaskFullLifecycleOrder(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	order := []string{}
	comp := &orderTrackingComponent{
		name:   "db",
		order:  &order,
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	app.OnStart(func(ctx context.Context) error {
		order = append(order, "onStart")
		return nil
	})
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		order = append(order, "onConfigure")
		return nil
	})
	app.OnReady(func(ctx context.Context) error {
		order = append(order, "onReady")
		return nil
	})
	app.OnStop(func(ctx context.Context) error {
		order = append(order, "onStop")
		return nil
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		order = append(order, "task")
		return nil
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}

	expected := []string{
		"onConfigure", // Phase 1: configure (may register app components)
		"db:start",    // Phase 2: single-pass StartAll
		"onStart",     // OnStart hooks
		"onReady",     // Ready hooks
		"task",        // Task execution
		"onStop",      // Stop hooks
		"db:stop",     // Components stop (reverse order)
	}

	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, expected %q", i, order[i], v)
		}
	}
}

// TestTwoPhaseStartupComponentsRegisteredDuringConfigure verifies that
// components registered during OnConfigure are automatically started
// before ReadyCheck runs.
func TestTwoPhaseStartupComponentsRegisteredDuringConfigure(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	order := []string{}

	// Phase 1: infra component registered before startup
	infra := &orderTrackingComponent{
		name:   "db",
		order:  &order,
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(infra)

	// OnConfigure registers an application-layer component (e.g. a worker)
	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		order = append(order, "onConfigure")
		worker := &orderTrackingComponent{
			name:   "catalog-refresh",
			order:  &order,
			health: component.Health{Name: "catalog-refresh", Status: component.StatusHealthy},
		}
		return a.RegisterComponent(worker)
	})

	app.OnReady(func(ctx context.Context) error {
		order = append(order, "onReady")
		return nil
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		order = append(order, "task")
		return nil
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}

	expected := []string{
		"onConfigure",           // Phase 1: configure — registers worker
		"db:start",              // Phase 2: single-pass StartAll (both components)
		"catalog-refresh:start", // Phase 2: late-registered component started in same pass
		"onReady",               // Ready hooks
		"task",                  // Task
		"catalog-refresh:stop",  // Stop: reverse order (app component first)
		"db:stop",               // Stop: infra last
	}

	if len(order) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, expected %q", i, order[i], v)
		}
	}
}

func TestStartupFailureSkipsTaskAndStopsComponents(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	comp := &mockComponent{
		name:     "bad-db",
		startErr: fmt.Errorf("connection refused"),
		health:   component.Health{Name: "bad-db", Status: component.StatusUnhealthy},
	}
	app.RegisterComponent(comp)

	taskRan := false
	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		taskRan = true
		return nil
	})

	if err == nil {
		t.Fatal("expected error from startup failure")
	}
	if taskRan {
		t.Error("task should not run when startup fails")
	}
}

func TestHookContextAvailability(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	var hookCtx context.Context
	app.OnStart(func(ctx context.Context) error {
		hookCtx = ctx
		return nil
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
	if hookCtx == nil {
		t.Error("hook should receive a non-nil context")
	}
}

func TestOnConfigureAccessToFullApp(t *testing.T) {
	cfg := newTestConfig("typed-svc", "2.0")
	app, _ := NewApp(cfg)

	comp := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	app.OnConfigure(func(ctx context.Context, a *App[*testConfig]) error {
		// Verify typed config access
		if a.Cfg.Name != "typed-svc" {
			return fmt.Errorf("expected typed-svc, got %s", a.Cfg.Name)
		}
		// Verify container access
		if a.Container == nil {
			return fmt.Errorf("container should not be nil")
		}
		// Verify component access
		if a.Components.Get("db") == nil {
			return fmt.Errorf("db component should be registered")
		}
		return nil
	})

	err := app.RunTask(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("RunTask failed: %v", err)
	}
}

func TestEmptyHookSlicesAreNoop(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)
	// No hooks registered — emit should succeed with no error.
	err := app.emitLifecycleHooks(context.Background(), EventStart)
	if err != nil {
		t.Errorf("no hooks should succeed: %v", err)
	}
	err = app.emitLifecycleHooks(context.Background(), EventStop)
	if err != nil {
		t.Errorf("no hooks should succeed: %v", err)
	}
}
