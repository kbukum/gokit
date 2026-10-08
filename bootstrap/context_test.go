package bootstrap

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
)

func TestStartupSummaryPreservesContext(t *testing.T) {
	t.Parallel()
	app, err := NewApp(newTestConfig("context-test", "1.0"))
	if err != nil {
		t.Fatal(err)
	}
	app.Summary.SetWriter(io.Discard)
	type key struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(t.Context(), key{}, "trace"), time.Minute)
	defer cancel()
	if err := app.RegisterComponent(&mockComponent{
		name: "health",
		healthFn: func(probe context.Context) component.Health {
			if probe.Value(key{}) != "trace" {
				t.Error("health probe lost startup context")
			}
			if _, ok := probe.Deadline(); !ok {
				t.Error("health probe lost startup deadline")
			}
			return component.Health{Name: "health", Status: component.StatusHealthy}
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := app.Startup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := app.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStartupCancellationDuringSummaryRollsBack(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"Startup", "Run", "RunTask"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			app := newQuietApp(t)
			cause := errors.New("canceled during summary")
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			summary := false
			app.OnReady(func(context.Context) error {
				summary = true
				return nil
			})
			comp := &mockComponent{name: "health", healthFn: func(context.Context) component.Health {
				if summary {
					cancel(cause)
				}
				return component.Health{Name: "health", Status: component.StatusHealthy}
			}}
			mustRegisterComponent(t, app, comp)
			taskRan := false
			var err error
			switch mode {
			case "Startup":
				err = app.Startup(ctx)
			case "Run":
				err = app.Run(ctx)
			case "RunTask":
				err = app.RunTask(ctx, func(context.Context) error {
					taskRan = true
					return nil
				})
			}
			t.Cleanup(func() {
				if err := app.Shutdown(context.Background()); err != nil {
					t.Errorf("Shutdown = %v", err)
				}
			})
			var failure *StartupError
			if !errors.As(err, &failure) || failure.Phase != PhaseReady || !errors.Is(err, cause) {
				t.Errorf("%s = %v, want a ready-phase StartupError preserving cancellation", mode, err)
			}
			if !comp.stopped || taskRan {
				t.Errorf("stopped = %v, task ran = %v", comp.stopped, taskRan)
			}
		})
	}
}

func TestStartupRollbackPreservesValuesWithoutCancellation(t *testing.T) {
	t.Parallel()
	app, err := NewApp(newTestConfig("rollback-test", "1.0"))
	if err != nil {
		t.Fatal(err)
	}
	type key struct{}
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), key{}, "trace"))
	defer cancel()
	cause := errors.New("startup failed")
	app.OnAfterStart(func(context.Context) error {
		cancel()
		return cause
	})
	called := false
	app.OnBeforeStop(func(cleanup context.Context) error {
		called = true
		if cleanup.Value(key{}) != "trace" || cleanup.Err() != nil {
			t.Errorf("rollback context lost values or retained cancellation: %v", cleanup.Err())
		}
		if _, ok := cleanup.Deadline(); !ok {
			t.Error("rollback must remain bounded")
		}
		return nil
	})
	if err := app.Startup(ctx); !errors.Is(err, cause) {
		t.Fatalf("lost startup failure: %v", err)
	}
	if !called {
		t.Fatal("rollback hook was not called")
	}
}
