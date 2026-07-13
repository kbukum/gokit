package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/di"
)

// failCloser returns an error from Close(); registering it in a real container
// exercises the container-close error path during shutdown.
type failCloser struct{}

func containerWithFailingClose(closeErr error) *di.Container {
	c := di.NewContainer()
	_ = di.RegisterCloseable(c, failCloser{}, func(context.Context, failCloser) error {
		return closeErr
	})
	return c
}

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

func TestContainerCloseErrorDuringShutdown(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	container := containerWithFailingClose(fmt.Errorf("container close failed"))
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
	container := containerWithFailingClose(fmt.Errorf("container close error"))
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

func TestRun_ExitsOnContextCancellation(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	comp := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // WaitForSignal returns immediately via ctx.Done

	if err := app.Run(ctx); err != nil {
		t.Fatalf("Run should exit cleanly on canceled context: %v", err)
	}
	if !comp.stopped {
		t.Error("component should be stopped after Run returns")
	}
}

func TestStartupThenShutdown(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	comp := &mockComponent{
		name:   "db",
		health: component.Health{Name: "db", Status: component.StatusHealthy},
	}
	app.RegisterComponent(comp)

	if err := app.Startup(context.Background()); err != nil {
		t.Fatalf("Startup failed: %v", err)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
	if !comp.stopped {
		t.Error("component should be stopped after Shutdown")
	}
}

func TestWaitForSignal_ReturnsNilOnContextCancel(t *testing.T) {
	cfg := newTestConfig("test", "1.0")
	app, _ := NewApp(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if sig := app.WaitForSignal(ctx); sig != nil {
		t.Fatalf("expected nil signal on context cancel, got %v", sig)
	}
}
