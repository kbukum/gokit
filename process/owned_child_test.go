package process

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestShutdownDoesNotReacquireCompletedGroup(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix process-group IDs")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	original := exec.Command(binary, "-test.run=^TestOwnedProcessChild$")
	original.Env = []string{"GOKIT_OWNED_CHILD=exit", "GORACE=atexit_sleep_ms=0"}
	original.Stdout, original.Stderr = io.Discard, io.Discard
	if err := original.Run(); err != nil {
		t.Fatal(err)
	}
	replacement := exec.Command(binary, "-test.run=^TestOwnedProcessChild$")
	replacement.Env = []string{"GOKIT_OWNED_CHILD=stubborn", "GORACE=atexit_sleep_ms=0"}
	replacement.Stdout, replacement.Stderr = io.Discard, io.Discard
	ConfigureSysProcAttr(replacement)
	if err := replacement.Start(); err != nil {
		t.Fatal(err)
	}
	replacementDone := make(chan struct{})
	go func() {
		_ = replacement.Wait()
		close(replacementDone)
	}()
	t.Cleanup(func() {
		if err := replacement.Process.Kill(); err != nil && !isExited(err) {
			t.Error(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		select {
		case <-replacementDone:
		case <-ctx.Done():
			t.Error("owned replacement fixture not reaped")
		}
	})
	// Model numeric PGID reuse with a live test-owned replacement and the old completed status.
	completed := &exec.Cmd{Process: replacement.Process, ProcessState: original.ProcessState}
	done := make(chan struct{})
	close(done)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	child := &ownedChild{
		cmd: completed, pid: replacement.Process.Pid,
		exited: done, done: done, groupReleased: true,
		exit: childExit{code: new(int)},
	}
	if _, err := shutdownOwned(ctx, child, DefaultLifecyclePolicy(), 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select {
	case <-replacementDone:
		t.Fatal("cleanup signaled a replacement process group")
	default:
	}
}
