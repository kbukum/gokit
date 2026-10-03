package process_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
)

func TestOwnedProcessChild(t *testing.T) {
	mode := os.Getenv("GOKIT_OWNED_CHILD")
	if mode == "" {
		return
	}
	if mode == "exit" {
		exitOwnedFixture()
		return
	}
	if mode == "parent" {
		runOwnedParent(t)
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	defer signal.Stop(signals)
	fmt.Print(strings.Repeat("a", 64*1024+1))
	fmt.Fprint(os.Stderr, strings.Repeat("b", 64*1024+1))
	fmt.Println("READY")
	for {
		<-signals
		if mode == "graceful" {
			return
		}
		if mode == "late-failure" {
			exitAfterGrace()
		}
	}
}

func startOwnedChild(t *testing.T, mode string) *process.PersistentProcess {
	t.Helper()
	return startOwnedChildWithPolicy(t, mode, process.DefaultLifecyclePolicy()).Process
}

func startOwnedChildWithPolicy(t *testing.T, mode string, policy process.LifecyclePolicy) *process.PersistentRun {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	run, err := process.StartPersistent(t.Context(), process.Command{
		Binary:         binary,
		Args:           []string{"-test.run=^TestOwnedProcessChild$"},
		EnvPolicy:      process.EnvEmpty,
		Env:            map[string]string{"GOKIT_OWNED_CHILD": mode, "GORACE": "atexit_sleep_ms=0"},
		MaxOutputBytes: 64 * 1024,
	}, process.PersistentConfig{
		Readiness:           process.ReadyOnOutput,
		OutputMarker:        "READY",
		ReadinessTimeout:    2 * time.Second,
		ShutdownGracePeriod: 50 * time.Millisecond,
		MaxCaptureBytes:     64 * 1024,
		Lifecycle:           policy,
	})
	if run != nil {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
			defer cancel()
			outcome, cleanupErr := run.Process.Shutdown(ctx)
			if !outcome.Complete {
				t.Errorf("owned child cleanup incomplete: %v", cleanupErr)
			} else if cleanupErr != nil {
				t.Logf("recorded owned shutdown outcome: %v", cleanupErr)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestOwnedGracefulAndForcedShutdown(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no SIGTERM graceful-shutdown contract")
	}
	for _, mode := range []string{"graceful", "stubborn"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			child := startOwnedChild(t, mode)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			start := time.Now()
			outcome, err := child.Shutdown(ctx)
			forced := mode == "stubborn"
			if (err != nil) != forced || outcome.Result == nil || outcome.Result.Forced != forced {
				t.Fatalf("forced=%t outcome=%+v err=%v", forced, outcome, err)
			}
			if len(outcome.Result.Stdout) > 64*1024 || len(outcome.Result.Stderr) > 64*1024 ||
				!outcome.Result.StdoutTruncated || !outcome.Result.StderrTruncated {
				t.Fatal("owned output exceeded its declared limit")
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("shutdown exceeded reaping ceiling")
			}
			if forced && outcome.Result.Success() {
				t.Fatal("forced shutdown reported success")
			}
			again, againErr := child.Shutdown(ctx)
			if again.Result != outcome.Result || (againErr != nil) != forced {
				t.Fatalf("repeated cleanup lost outcome: %+v %v", again, againErr)
			}
		})
	}
}

func TestOwnedCanceledShutdown(t *testing.T) {
	t.Parallel()
	child := startOwnedChild(t, "stubborn")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	outcome, err := child.Shutdown(ctx)
	if !errors.Is(err, context.Canceled) || outcome.Result == nil || !outcome.Result.Canceled || !outcome.Result.Forced {
		t.Fatalf("cancellation outcome: %+v %v", outcome, err)
	}
}

func exitAfterGrace() {
	timer := time.NewTimer(100 * time.Millisecond)
	<-timer.C
	os.Exit(7)
}

func exitOwnedFixture() {
	fmt.Println("READY")
	os.Exit(0)
}

func TestShutdownReportsExitFailureAfterGrace(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no SIGTERM graceful-shutdown contract")
	}
	policy := process.DefaultLifecyclePolicy()
	policy.GracePeriod = 50 * time.Millisecond
	policy.KillAfterGrace = false
	t.Run("persistent", func(t *testing.T) {
		t.Parallel()
		child := startOwnedChildWithPolicy(t, "late-failure", policy).Process
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		outcome, err := child.Shutdown(ctx)
		assertLateExitFailure(t, outcome, err)
		again, againErr := child.Shutdown(ctx)
		assertLateExitFailure(t, again, againErr)
	})
	t.Run("supervised", func(t *testing.T) {
		t.Parallel()
		supervisor, _, _ := startSupervisedOwnedChild(t, "late-failure", policy)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		report, err := supervisor.Shutdown(ctx, "late failure")
		if len(report.Children) != 1 || !report.Children[0].Reaped {
			t.Fatalf("incomplete report: %+v", report)
		}
		assertLateExitFailure(t, report.Children[0].ShutdownOutcome, err)
		again, againErr := supervisor.Shutdown(ctx, "repeat")
		assertLateExitFailure(t, again.Children[0].ShutdownOutcome, againErr)
	})
}

func assertLateExitFailure(t *testing.T, outcome process.ShutdownOutcome, err error) {
	t.Helper()
	if err == nil || outcome.Result == nil || outcome.Result.ExitCode == nil ||
		*outcome.Result.ExitCode != 7 || outcome.Result.Forced {
		t.Fatalf("late failed exit certified success: %+v %v", outcome, err)
	}
}
