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
