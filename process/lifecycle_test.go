package process_test

import (
	"context"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
)

func TestDefaultLifecyclePolicy(t *testing.T) {
	t.Parallel()

	p := process.DefaultLifecyclePolicy()
	if p.GracePeriod != process.DefaultGracePeriod {
		t.Fatalf("GracePeriod = %v, want %v", p.GracePeriod, process.DefaultGracePeriod)
	}
	if !p.IsolateProcessGroup || !p.TerminateDescendants || !p.KillAfterGrace {
		t.Fatalf("expected all lifecycle toggles enabled, got %+v", p)
	}
}

func TestRunLifecyclePolicyGracePeriod(t *testing.T) {
	t.Parallel()

	// A process that ignores SIGTERM must still be killed after the grace period.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := process.Run(ctx, process.Command{
		Binary: "sh",
		Args:   []string{"-c", "trap '' TERM; sleep 60"},
		Lifecycle: &process.LifecyclePolicy{
			GracePeriod:          200 * time.Millisecond,
			IsolateProcessGroup:  true,
			TerminateDescendants: true,
			KillAfterGrace:       true,
		},
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from context timeout")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("kill escalation too slow: %v", elapsed)
	}
}

func TestRunTimedOutResult(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	result, err := process.Run(ctx, process.Command{
		Binary:      "sleep",
		Args:        []string{"30"},
		GracePeriod: 100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !result.TimedOut {
		t.Fatalf("expected TimedOut=true, got %+v", result)
	}
	if result.Canceled {
		t.Fatal("expected Canceled=false on timeout")
	}
}

func TestRunCanceledResult(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := process.Run(ctx, process.Command{
		Binary:      "sleep",
		Args:        []string{"30"},
		GracePeriod: 100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !result.Canceled {
		t.Fatalf("expected Canceled=true, got %+v", result)
	}
	if result.TimedOut {
		t.Fatal("expected TimedOut=false on cancel")
	}
}
