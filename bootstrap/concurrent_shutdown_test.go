package bootstrap

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
	componenttest "github.com/kbukum/gokit/component/testutil"
)

const raceWait = 2 * time.Second

func waitFor[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(raceWait):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

// A hook that ignores cancellation must not let startup create resources after Shutdown.
func TestShutdownDuringStartupWaitsAndRollsBack(t *testing.T) {
	app := newQuietApp(t)
	var starts atomic.Int32
	mustRegisterComponent(t, app, &componenttest.Component{ComponentName: "db", StartFunc: func(context.Context) error {
		starts.Add(1)
		return nil
	}})
	entered, release := make(chan struct{}), make(chan struct{})
	app.OnConfigure(func(context.Context, *App[*testConfig]) error {
		close(entered)
		<-release
		return nil
	})
	startupErr := make(chan error, 1)
	go func() { startupErr <- app.Startup(context.Background()) }()
	waitFor(t, entered, "configure hook")

	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- app.Shutdown(context.Background()) }()
	select {
	case err := <-shutdownErr:
		t.Fatalf("Shutdown returned %v while startup was still running", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)

	var failure *StartupError
	if err := waitFor(t, startupErr, "startup"); !errors.As(err, &failure) || failure.Phase != PhaseConfigure || !errors.Is(err, ErrShutdownRequested) {
		t.Fatalf("Startup error = %v, want configure-phase StartupError caused by ErrShutdownRequested", err)
	}
	if err := waitFor(t, shutdownErr, "shutdown"); err != nil {
		t.Fatalf("Shutdown error = %v, want recorded clean rollback", err)
	}
	if starts.Load() != 0 {
		t.Fatal("component started after shutdown was requested")
	}
}

func TestShutdownCancelsBlockingStartupPhase(t *testing.T) {
	app := newQuietApp(t)
	entered := make(chan struct{})
	app.OnConfigure(func(ctx context.Context, _ *App[*testConfig]) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	startupErr := make(chan error, 1)
	go func() { startupErr <- app.Startup(context.Background()) }()
	waitFor(t, entered, "configure hook")
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown error = %v", err)
	}
	if err := waitFor(t, startupErr, "startup"); !errors.Is(err, context.Canceled) || !errors.Is(err, ErrShutdownRequested) {
		t.Fatalf("Startup error = %v, want the canceled phase and ErrShutdownRequested", err)
	}
}

// A ready hook must not run once Shutdown interrupts the ready check.
func TestShutdownDuringReadyCheckSkipsReadyHooks(t *testing.T) {
	app := newQuietApp(t)
	entered, release := make(chan struct{}), make(chan struct{})
	mustRegisterComponent(t, app, &componenttest.Component{ComponentName: "db", HealthFunc: func(context.Context) component.Health {
		close(entered)
		<-release
		return component.Health{Name: "db", Status: component.StatusHealthy}
	}})
	var readyRan atomic.Bool
	app.OnReady(func(context.Context) error {
		readyRan.Store(true)
		return nil
	})
	startupErr := make(chan error, 1)
	go func() { startupErr <- app.Startup(context.Background()) }()
	waitFor(t, entered, "ready check")
	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- app.Shutdown(context.Background()) }()
	waitForLifecycleCancel(t, app)
	close(release)

	var failure *StartupError
	if err := waitFor(t, startupErr, "startup"); !errors.As(err, &failure) || failure.Phase != PhaseReady || !errors.Is(err, ErrShutdownRequested) {
		t.Fatalf("Startup error = %v, want ready-phase StartupError caused by ErrShutdownRequested", err)
	}
	if err := waitFor(t, shutdownErr, "shutdown"); err != nil {
		t.Fatalf("Shutdown error = %v", err)
	}
	if readyRan.Load() {
		t.Fatal("ready hook ran after shutdown was requested")
	}
}

// Teardown must wait until an active RunTask task has returned.
func TestShutdownWaitsForActiveTask(t *testing.T) {
	app := newQuietApp(t)
	var stopped atomic.Bool
	mustRegisterComponent(t, app, &componenttest.Component{ComponentName: "db", StopFunc: func(context.Context) error {
		stopped.Store(true)
		return nil
	}})
	started, unwinding, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var stoppedDuringTask atomic.Bool
	taskErr := make(chan error, 1)
	go func() {
		taskErr <- app.RunTask(context.Background(), func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			close(unwinding)
			<-release
			stoppedDuringTask.Store(stopped.Load())
			return ctx.Err()
		})
	}()
	waitFor(t, started, "task")
	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- app.Shutdown(context.Background()) }()
	waitFor(t, unwinding, "task cancellation")
	select {
	case err := <-shutdownErr:
		t.Fatalf("Shutdown returned %v while the task was still running", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := waitFor(t, shutdownErr, "shutdown"); err != nil {
		t.Fatalf("Shutdown error = %v", err)
	}
	if err := waitFor(t, taskErr, "RunTask to return"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunTask error = %v, want canceled task", err)
	}
	if stoppedDuringTask.Load() {
		t.Fatal("components were stopped while the task was still running")
	}
}

func TestShutdownWaitForTaskIsBounded(t *testing.T) {
	app := newQuietApp(t)
	started, release := make(chan struct{}), make(chan struct{})
	taskErr := make(chan error, 1)
	go func() {
		taskErr <- app.RunTask(context.Background(), func(context.Context) error {
			close(started)
			<-release
			return nil
		})
	}()
	waitFor(t, started, "task")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var shutdownFailure *ShutdownError
	if err := app.Shutdown(ctx); !errors.As(err, &shutdownFailure) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want ShutdownError with deadline", err)
	}
	close(release)
	if err := waitFor(t, taskErr, "RunTask to return"); err != nil {
		t.Fatalf("RunTask error = %v", err)
	}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("later Shutdown = %v, want recorded clean teardown", err)
	}
}

// waitForLifecycleCancel blocks until Shutdown has canceled the startup context.
func waitForLifecycleCancel(t *testing.T, app *App[*testConfig]) {
	t.Helper()
	deadline := time.Now().Add(raceWait)
	for time.Now().Before(deadline) {
		app.lifecycleMu.Lock()
		closed := app.stopping != nil && isClosed(app.stopping)
		app.lifecycleMu.Unlock()
		if closed {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for Shutdown to cancel startup")
}
