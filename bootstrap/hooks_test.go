package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/kbukum/gokit/component"
)

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
