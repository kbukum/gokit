package bootstrap

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	componenttest "github.com/kbukum/gokit/component/testutil"
)

func TestReadinessRechecksLifecycleAfterHealth(t *testing.T) {
	t.Parallel()
	app := newAdminApp(t, AdminConfig{})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var armed atomic.Bool
	entered, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	mustRegisterComponent(t, app, &componenttest.Component{
		ComponentName: "blocking-health",
		HealthFunc: func(ctx context.Context) component.Health {
			if armed.Load() {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
				}
			}
			return component.Health{Name: "blocking-health", Status: component.StatusHealthy}
		},
	})
	draining, finish := make(chan struct{}), make(chan struct{})
	finishStop := sync.OnceFunc(func() { close(finish) })
	defer finishStop()
	app.OnBeforeStop(func(ctx context.Context) error {
		close(draining)
		select {
		case <-finish:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err := app.Startup(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := app.Shutdown(cleanup); err != nil {
			t.Error(err)
		}
	})
	armed.Store(true)
	result := make(chan Readiness, 1)
	go func() { result <- app.readiness(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("health probe did not start")
	}
	stopped := make(chan error, 1)
	go func() { stopped <- app.Shutdown(ctx) }()
	select {
	case <-draining:
	case <-ctx.Done():
		t.Fatal("shutdown did not begin")
	}
	unblock()
	select {
	case got := <-result:
		if got.Status != ReadinessDraining || len(got.Components) != 0 {
			t.Errorf("overlapping readiness = %+v, want draining without stale health", got)
		}
	case <-ctx.Done():
		t.Error("health probe did not finish")
	}
	finishStop()
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
}
