package process_test

import (
	"context"
	"strings"
	"testing"
	"time"

	goerrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/process"
)

func TestStartPersistentReadyImmediate(t *testing.T) {
	t.Parallel()

	run, err := process.StartPersistent(t.Context(), process.Command{
		Binary: "sleep",
		Args:   []string{"60"},
	}, process.DefaultPersistentConfig())
	if err != nil {
		t.Fatalf("StartPersistent: %v", err)
	}
	if run.Process.Pid() <= 0 {
		t.Fatalf("Pid() = %d, want > 0", run.Process.Pid())
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	outcome, err := run.Process.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if outcome.Result == nil {
		t.Fatal("expected a result from Shutdown")
	}
}

func TestStartPersistentReadyImmediateHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	run, err := process.StartPersistent(ctx, process.Command{
		Binary: "sleep",
		Args:   []string{"60"},
	}, process.DefaultPersistentConfig())
	if err == nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		_, _ = run.Process.Shutdown(shutdownCtx)
		t.Fatal("expected canceled context to abort persistent startup")
	}
	assertProcessAppErrorCode(t, err, goerrors.ErrCodeCanceled)
}

func TestStartPersistentReadyAfterDelayClassifiesContextDeadline(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	_, err := process.StartPersistent(ctx, process.Command{
		Binary: "sleep",
		Args:   []string{"60"},
	}, process.PersistentConfig{
		Readiness:           process.ReadyAfterDelay,
		ReadyDelay:          time.Second,
		ReadinessTimeout:    time.Second,
		ShutdownGracePeriod: 50 * time.Millisecond,
		Lifecycle: process.LifecyclePolicy{
			GracePeriod:          50 * time.Millisecond,
			IsolateProcessGroup:  true,
			TerminateDescendants: true,
			KillAfterGrace:       true,
		},
	})
	assertProcessAppErrorCode(t, err, goerrors.ErrCodeTimeout)
}

func TestStartPersistentReadyOnOutput(t *testing.T) {
	t.Parallel()

	cfg := process.DefaultPersistentConfig()
	cfg.Readiness = process.ReadyOnOutput
	cfg.OutputMarker = "READY"
	cfg.ReadinessTimeout = 5 * time.Second

	run, err := process.StartPersistent(t.Context(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "echo READY; sleep 60"},
	}, cfg)
	if err != nil {
		t.Fatalf("StartPersistent: %v", err)
	}
	defer func() { _, _ = run.Process.Shutdown(t.Context()) }()

	if !strings.Contains(string(run.Startup.Stdout), "READY") {
		t.Fatalf("startup stdout = %q, want to contain READY", run.Startup.Stdout)
	}
}

func TestStartPersistentReadinessTimedOut(t *testing.T) {
	t.Parallel()

	cfg := process.DefaultPersistentConfig()
	cfg.Readiness = process.ReadyOnOutput
	cfg.OutputMarker = "NEVER"
	cfg.ReadinessTimeout = 150 * time.Millisecond

	_, err := process.StartPersistent(t.Context(), process.Command{
		Binary: "sleep",
		Args:   []string{"60"},
	}, cfg)
	if err == nil {
		t.Fatal("expected readiness timeout error")
	}
	kind, ok := process.StartErrorKind(err)
	if !ok || kind != process.PersistentStartReadinessTimedOut {
		t.Fatalf("StartErrorKind = %q (%v), want readiness_timed_out", kind, ok)
	}
}

func TestStartPersistentExitedBeforeReadiness(t *testing.T) {
	t.Parallel()

	cfg := process.DefaultPersistentConfig()
	cfg.Readiness = process.ReadyOnOutput
	cfg.OutputMarker = "NEVER"
	cfg.ReadinessTimeout = 5 * time.Second

	_, err := process.StartPersistent(t.Context(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "exit 3"},
	}, cfg)
	if err == nil {
		t.Fatal("expected exited-before-readiness error")
	}
	kind, ok := process.StartErrorKind(err)
	if !ok || kind != process.PersistentStartExitedBeforeReadiness {
		t.Fatalf("StartErrorKind = %q (%v), want exited_before_readiness", kind, ok)
	}
}

func TestStartPersistentSpawnFailed(t *testing.T) {
	t.Parallel()

	_, err := process.StartPersistent(t.Context(), process.Command{
		Binary: "nonexistent_binary_xyz_99999",
	}, process.DefaultPersistentConfig())
	if err == nil {
		t.Fatal("expected spawn failure")
	}
	kind, ok := process.StartErrorKind(err)
	if !ok || kind != process.PersistentStartSpawnFailed {
		t.Fatalf("StartErrorKind = %q (%v), want spawn_failed", kind, ok)
	}
	appErr, isApp := goerrors.AsAppError(err)
	if !isApp || appErr.Code != goerrors.ErrCodeNotFound {
		t.Fatalf("spawn failure code = %v, want NOT_FOUND", appErr)
	}
}

func TestStartPersistentWaitNaturalExit(t *testing.T) {
	t.Parallel()

	cfg := process.DefaultPersistentConfig()
	cfg.Readiness = process.ReadyOnOutput
	cfg.OutputMarker = "up"
	cfg.ReadinessTimeout = 5 * time.Second

	run, err := process.StartPersistent(t.Context(), process.Command{
		Binary: "sh",
		Args:   []string{"-c", "echo up; exit 0"},
	}, cfg)
	if err != nil {
		t.Fatalf("StartPersistent: %v", err)
	}
	if err := run.Process.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	// A second lifecycle call must be rejected.
	if _, err := run.Process.Shutdown(t.Context()); err == nil {
		t.Fatal("expected conflict on Shutdown after Wait")
	}
}

func TestStartPersistentEmptyMarkerRejected(t *testing.T) {
	t.Parallel()

	cfg := process.DefaultPersistentConfig()
	cfg.Readiness = process.ReadyOnOutput

	_, err := process.StartPersistent(t.Context(), process.Command{Binary: "sleep", Args: []string{"1"}}, cfg)
	if err == nil {
		t.Fatal("expected invalid input error for empty marker")
	}
	if appErr, ok := goerrors.AsAppError(err); !ok || appErr.Code != goerrors.ErrCodeInvalidInput {
		t.Fatalf("error = %v, want INVALID_INPUT", err)
	}
}

func assertProcessAppErrorCode(t *testing.T, err error, want goerrors.ErrorCode) {
	t.Helper()
	appErr, ok := goerrors.AsAppError(err)
	if !ok {
		t.Fatalf("error = %T, want *errors.AppError", err)
	}
	if appErr.Code != want {
		t.Fatalf("code = %s, want %s", appErr.Code, want)
	}
}
