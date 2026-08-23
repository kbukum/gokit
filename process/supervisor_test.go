package process_test

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
)

func startSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	process.ConfigureSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleeper: %v", err)
	}
	return cmd
}

func TestSupervisorShutdownTerminatesTrackedChildren(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.DefaultLifecyclePolicy())
	c1 := startSleeper(t)
	c2 := startSleeper(t)
	sup.Track(c1)
	sup.Track(c2)

	if sup.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", sup.Len())
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := sup.Shutdown(ctx, "test teardown"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if sup.Len() != 0 {
		t.Fatalf("Len() after shutdown = %d, want 0", sup.Len())
	}

	// Both children must be reaped: their process state is set after Wait returns.
	if c1.ProcessState == nil || c2.ProcessState == nil {
		t.Fatal("expected both children reaped after Shutdown")
	}
}

func TestSupervisorShutdownIdempotent(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.DefaultLifecyclePolicy())
	c := startSleeper(t)
	sup.Track(c)

	if err := sup.Shutdown(t.Context(), "first"); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if err := sup.Shutdown(t.Context(), "second"); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

func TestSupervisorReleaseUntracks(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.DefaultLifecyclePolicy())
	c := startSleeper(t)
	h := sup.Track(c)
	if sup.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", sup.Len())
	}
	sup.Release(h)
	if sup.Len() != 0 {
		t.Fatalf("Len() after Release = %d, want 0", sup.Len())
	}

	// Caller owns reaping now.
	_ = process.KillGroup(c)
	_ = c.Wait()
}

func TestSupervisorShutdownEmpty(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.LifecyclePolicy{})
	if err := sup.Shutdown(t.Context(), "noop"); err != nil {
		t.Fatalf("Shutdown on empty supervisor: %v", err)
	}
}

func TestSupervisorTrackNilAndUnstarted(t *testing.T) {
	t.Parallel()

	sup := process.NewSupervisor(process.DefaultLifecyclePolicy())
	if h := sup.Track(nil); h != 0 {
		t.Fatalf("Track(nil) = %d, want 0", h)
	}
	if h := sup.Track(exec.Command("echo")); h != 0 {
		t.Fatalf("Track(unstarted) = %d, want 0", h)
	}
	if h := sup.TrackPid(0); h != 0 {
		t.Fatalf("TrackPid(0) = %d, want 0", h)
	}
}
