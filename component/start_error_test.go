package component_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	componenttest "github.com/kbukum/gokit/component/testutil"
)

func TestStartAllReturnsRollbackFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		start func(*component.Registry, context.Context) error
	}{
		{"sequential", (*component.Registry).StartAll},
		{"concurrent", (*component.Registry).StartAllConcurrent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			startErr, stopErr := errors.New("bind refused"), errors.New("release failed")
			registry := component.NewRegistry()
			first := make(chan struct{})
			mustRegister(t, registry, &componenttest.Component{
				ComponentName: "first",
				StartFunc:     func(context.Context) error { close(first); return nil },
				StopFunc:      func(context.Context) error { return stopErr },
			})
			mustRegister(t, registry, &componenttest.Component{
				ComponentName: "second",
				StartFunc: func(context.Context) error {
					<-first // fail only after the first component is running, so rollback is deterministic
					return startErr
				},
			})
			err := tc.start(registry, t.Context())
			var failure *component.StartError
			if !errors.As(err, &failure) {
				t.Fatalf("StartAll error = %T %v, want *component.StartError", err, err)
			}
			if failure.Component != "second" || !errors.Is(failure.Cause, startErr) {
				t.Fatalf("start failure = %q %v", failure.Component, failure.Cause)
			}
			if !errors.Is(failure.Rollback, stopErr) || errors.Is(failure.Cause, stopErr) {
				t.Fatalf("rollback failure = %v", failure.Rollback)
			}
			if !errors.Is(err, startErr) || !errors.Is(err, stopErr) {
				t.Fatalf("joined chain lost a cause: %v", err)
			}
		})
	}
}

func TestStartAllReportsTimeoutCleanupFailure(t *testing.T) {
	t.Parallel()
	cleanupErr := errors.New("partial release failed")
	registry := component.NewRegistryWithConfig(component.RegistryConfig{StartTimeout: 10 * time.Millisecond, StopTimeout: time.Second})
	mustRegister(t, registry, &componenttest.Component{
		ComponentName: "slow",
		StartFunc: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
		StopFunc: func(context.Context) error { return cleanupErr },
	})
	err := registry.StartAll(context.Background())
	var failure *component.StartError
	if !errors.As(err, &failure) || !errors.Is(failure.Cause, context.DeadlineExceeded) || !errors.Is(failure.Rollback, cleanupErr) {
		t.Fatalf("StartAll error = %v", err)
	}
}

func TestStartAllCleanRollbackHasNoRollbackError(t *testing.T) {
	t.Parallel()
	startErr := errors.New("bind refused")
	registry := component.NewRegistry()
	mustRegister(t, registry, &componenttest.Component{ComponentName: "first"})
	mustRegister(t, registry, &componenttest.Component{
		ComponentName: "second",
		StartFunc:     func(context.Context) error { return startErr },
	})
	err := registry.StartAll(t.Context())
	var failure *component.StartError
	if !errors.As(err, &failure) || failure.Rollback != nil {
		t.Fatalf("StartAll error = %v", err)
	}
	if got, want := err.Error(), "failed to start second: bind refused"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestRegisterRejectsEmptyName(t *testing.T) {
	t.Parallel()
	if err := component.NewRegistry().Register(&componenttest.Component{}); err == nil {
		t.Fatal("empty component name must be rejected")
	}
}

func mustRegister(t *testing.T, registry *component.Registry, c component.Component) {
	t.Helper()
	if err := registry.Register(c); err != nil {
		t.Fatal(err)
	}
}
