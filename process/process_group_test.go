package process_test

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
)

func TestGroupSignalsNilSafe(t *testing.T) {
	t.Parallel()

	// A nil command or unstarted process must be a no-op, not a panic.
	if err := process.TerminateGroup(nil); err != nil {
		t.Fatalf("TerminateGroup(nil) = %v", err)
	}
	if err := process.KillGroup(exec.Command("echo")); err != nil {
		t.Fatalf("KillGroup(unstarted) = %v", err)
	}
	if err := process.InterruptGroup(nil); err != nil {
		t.Fatalf("InterruptGroup(nil) = %v", err)
	}
}

func TestTerminateGroupStopsProcess(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("process-group termination semantics differ on Windows")
	}

	cmd := exec.Command("sleep", "60")
	process.ConfigureSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := process.TerminateGroup(cmd); err != nil {
		t.Fatalf("TerminateGroup: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = process.KillGroup(cmd)
		t.Fatal("process did not terminate after TerminateGroup")
	}
}
