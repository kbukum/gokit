package bootstrap

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kbukum/gokit/component"
	componenttest "github.com/kbukum/gokit/component/testutil"
	"github.com/kbukum/gokit/logging"
)

func TestRunTaskReportsTaskAndShutdownFailures(t *testing.T) {
	taskErr, stopErr := errors.New("task failed"), errors.New("cleanup failed")
	app := newQuietApp(t)
	app.OnBeforeStop(func(context.Context) error { return stopErr })

	err := app.RunTask(t.Context(), func(context.Context) error { return taskErr })

	var failure *TaskError
	if !errors.As(err, &failure) {
		t.Fatalf("RunTask error = %T %v, want *TaskError", err, err)
	}
	if !errors.Is(failure.Cause, taskErr) || errors.Is(failure.Cause, stopErr) {
		t.Fatalf("task cause = %v", failure.Cause)
	}
	var shutdown *ShutdownError
	if !errors.As(failure.Shutdown, &shutdown) || !errors.Is(shutdown, stopErr) {
		t.Fatalf("task shutdown = %v", failure.Shutdown)
	}
	if !errors.Is(err, taskErr) || !errors.Is(err, stopErr) {
		t.Fatalf("combined chain lost a cause: %v", err)
	}
}

func TestRunTaskFailureWithCleanShutdown(t *testing.T) {
	taskErr := errors.New("task failed")
	app := newQuietApp(t)
	err := app.RunTask(t.Context(), func(context.Context) error { return taskErr })
	var failure *TaskError
	if !errors.As(err, &failure) || !errors.Is(failure.Cause, taskErr) || failure.Shutdown != nil {
		t.Fatalf("RunTask error = %v", err)
	}
}

func TestRunTaskShutdownFailureAfterSuccessfulTask(t *testing.T) {
	stopErr := errors.New("cleanup failed")
	app := newQuietApp(t)
	app.OnAfterStop(func(context.Context) error { return stopErr })
	err := app.RunTask(t.Context(), func(context.Context) error { return nil })
	var shutdown *ShutdownError
	if !errors.As(err, &shutdown) || !errors.Is(err, stopErr) {
		t.Fatalf("RunTask error = %T %v, want *ShutdownError", err, err)
	}
	var failure *TaskError
	if errors.As(err, &failure) {
		t.Fatal("a successful task must not report a task failure")
	}
}

func TestStartupReturnsRollbackFailure(t *testing.T) {
	hookErr, stopErr := errors.New("wiring failed"), errors.New("release failed")
	app := newQuietApp(t)
	mustRegisterComponent(t, app, &componenttest.Component{
		ComponentName: "db",
		StopFunc:      func(context.Context) error { return stopErr },
	})
	app.OnAfterStart(func(context.Context) error { return hookErr })

	err := app.Startup(t.Context())

	var failure *StartupError
	if !errors.As(err, &failure) {
		t.Fatalf("Startup error = %T %v, want *StartupError", err, err)
	}
	if failure.Phase != PhaseAfterStart || !errors.Is(failure.Cause, hookErr) || errors.Is(failure.Cause, stopErr) {
		t.Fatalf("startup failure = %s %v", failure.Phase, failure.Cause)
	}
	var rollback *ShutdownError
	if !errors.As(failure.Rollback, &rollback) || !errors.Is(rollback, stopErr) {
		t.Fatalf("rollback = %v", failure.Rollback)
	}
	if !errors.Is(err, hookErr) || !errors.Is(err, stopErr) {
		t.Fatalf("combined chain lost a cause: %v", err)
	}
}

func TestStartupKeepsComponentRollbackSeparate(t *testing.T) {
	startErr, stopErr := errors.New("bind refused"), errors.New("release failed")
	app := newQuietApp(t)
	mustRegisterComponent(t, app, &componenttest.Component{
		ComponentName: "first",
		StopFunc:      func(context.Context) error { return stopErr },
	})
	mustRegisterComponent(t, app, &componenttest.Component{
		ComponentName: "second",
		StartFunc:     func(context.Context) error { return startErr },
	})

	err := app.Startup(t.Context())

	var failure *StartupError
	if !errors.As(err, &failure) || failure.Phase != PhaseStart || failure.Rollback != nil {
		t.Fatalf("Startup error = %v", err)
	}
	var start *component.StartError
	if !errors.As(failure.Cause, &start) || start.Component != "second" || !errors.Is(start.Cause, startErr) || !errors.Is(start.Rollback, stopErr) {
		t.Fatalf("component start failure = %v", failure.Cause)
	}
}

func TestStartupPhasesAreReported(t *testing.T) {
	cause := errors.New("hook failed")
	for _, tc := range []struct {
		phase    Phase
		register func(*App[*testConfig])
	}{
		{PhaseConfigure, func(a *App[*testConfig]) {
			a.OnConfigure(func(context.Context, *App[*testConfig]) error { return cause })
		}},
		{PhaseBeforeStart, func(a *App[*testConfig]) { a.OnBeforeStart(func(context.Context) error { return cause }) }},
		{PhaseAfterStart, func(a *App[*testConfig]) { a.OnAfterStart(func(context.Context) error { return cause }) }},
		{PhaseReady, func(a *App[*testConfig]) { a.OnReady(func(context.Context) error { return cause }) }},
	} {
		t.Run(string(tc.phase), func(t *testing.T) {
			app := newQuietApp(t)
			tc.register(app)
			err := app.Startup(t.Context())
			var failure *StartupError
			if !errors.As(err, &failure) || failure.Phase != tc.phase || !errors.Is(failure.Cause, cause) || failure.Rollback != nil {
				t.Fatalf("Startup error = %v", err)
			}
			if !strings.Contains(err.Error(), string(tc.phase)) {
				t.Fatalf("message %q does not name the phase", err.Error())
			}
		})
	}
}

func TestAppRunsOneLifecycle(t *testing.T) {
	app := newQuietApp(t)
	if err := app.RunTask(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	ran := false
	err := app.RunTask(t.Context(), func(context.Context) error { ran = true; return nil })
	if !errors.Is(err, ErrLifecycleUsed) || ran {
		t.Fatalf("second RunTask = %v ran=%v, want ErrLifecycleUsed", err, ran)
	}
	if err := app.Startup(t.Context()); !errors.Is(err, ErrLifecycleUsed) {
		t.Fatalf("Startup after shutdown = %v", err)
	}
	if err := app.Run(t.Context()); !errors.Is(err, ErrLifecycleUsed) {
		t.Fatalf("Run after shutdown = %v", err)
	}
}

func TestStartupRejectsSecondStart(t *testing.T) {
	app := newQuietApp(t)
	if err := app.Startup(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Shutdown(context.Background()) })
	if err := app.Startup(t.Context()); !errors.Is(err, ErrLifecycleUsed) {
		t.Fatalf("second Startup = %v", err)
	}
}

func TestShutdownRunsOnceAndReturnsRecordedOutcome(t *testing.T) {
	stopErr := errors.New("cleanup failed")
	app := newQuietApp(t)
	calls := 0
	app.OnAfterStop(func(context.Context) error { calls++; return stopErr })
	if err := app.Startup(t.Context()); err != nil {
		t.Fatal(err)
	}
	first := app.Shutdown(t.Context())
	second := app.Shutdown(t.Context())
	if calls != 1 || !errors.Is(first, stopErr) || first != second { //nolint:errorlint // identity: the recorded outcome is returned unchanged
		t.Fatalf("calls=%d first=%v second=%v", calls, first, second)
	}
}

func TestShutdownAfterFailedStartupReturnsRollbackOutcome(t *testing.T) {
	stopErr := errors.New("release failed")
	app := newQuietApp(t)
	calls := 0
	mustRegisterComponent(t, app, &componenttest.Component{
		ComponentName: "db",
		StopFunc:      func(context.Context) error { calls++; return stopErr },
	})
	app.OnReady(func(context.Context) error { return errors.New("not ready") })
	var failure *StartupError
	if err := app.Startup(t.Context()); !errors.As(err, &failure) {
		t.Fatal(err)
	}
	if err := app.Shutdown(t.Context()); err != failure.Rollback || calls != 1 { //nolint:errorlint // identity: the recorded outcome is returned unchanged
		t.Fatalf("Shutdown after rollback = %v calls=%d", err, calls)
	}
}

func TestOwnedLoggerIsReleasedAfterShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	cfg := newTestConfig("owned", "1.0")
	cfg.Logging = logging.Config{Level: "info", Format: "json", Output: logging.OutputFile(path)}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatal(err)
	}
	app.Summary.SetWriter(io.Discard)
	if err := app.RunTask(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	app.Logger.Info("after release")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Application shutdown complete") || strings.Contains(string(data), "after release") {
		t.Fatalf("owned logger was not released after shutdown: %q", data)
	}
}

func TestBorrowedLoggerStaysOpenAfterShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	logger, err := logging.New(&logging.Config{Level: "info", Format: "json", Output: logging.OutputFile(path)}, "borrowed")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	app, err := NewApp(newTestConfig("borrowed", "1.0"), WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	app.Summary.SetWriter(io.Discard)
	if err := app.RunTask(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	logger.Info("borrowed still writes")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "borrowed still writes") {
		t.Fatalf("borrowed logger was closed by the app: %q", data)
	}
}

func newQuietApp(t *testing.T) *App[*testConfig] {
	t.Helper()
	app, err := NewApp(newTestConfig("test", "1.0"), WithLogger(logging.NewDefault("test")))
	if err != nil {
		t.Fatal(err)
	}
	app.Summary.SetWriter(io.Discard)
	return app
}

func mustRegisterComponent(t *testing.T, app *App[*testConfig], c component.Component) {
	t.Helper()
	if err := app.RegisterComponent(c); err != nil {
		t.Fatal(err)
	}
}
