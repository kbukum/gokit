package process_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
)

// The fixture parent exits zero without waiting for its SIGTERM-ignoring descendant.
func runOwnedParent(t *testing.T) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(binary, "-test.run=^TestOwnedProcessChild$")
	child.Env = []string{"GOKIT_OWNED_CHILD=stubborn", "GORACE=atexit_sleep_ms=0"}
	child.Stderr = io.Discard
	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(io.LimitReader(out, 66*1024)).ReadString('\n')
	if err != nil || !strings.Contains(line, "READY") {
		if killErr := child.Process.Kill(); killErr != nil {
			t.Error(killErr)
		}
		if waitErr := child.Wait(); waitErr == nil {
			t.Error("forced fixture child unexpectedly exited successfully")
		}
		t.Fatalf("descendant readiness: %v", err)
	}
	fmt.Printf("DESCENDANT:%d\nREADY\n", child.Process.Pid)
	<-signals
	signal.Stop(signals)
	os.Exit(0)
}

func TestShutdownOwnsDescendantsAfterParentExit(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not implement Unix process-group signaling")
	}
	t.Run("persistent", func(t *testing.T) {
		t.Parallel()
		parent := startOwnedChildWithPolicy(t, "parent", process.DefaultLifecyclePolicy())
		descendant := ownedDescendant(t, string(parent.Startup.Stdout))
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		outcome, err := parent.Process.Shutdown(ctx)
		assertDescendantShutdown(t, outcome, err)
		assertOwnedProcessGone(t, descendant)
	})
	t.Run("supervised", func(t *testing.T) {
		t.Parallel()
		policy := process.DefaultLifecyclePolicy()
		policy.GracePeriod = 50 * time.Millisecond
		supervisor, _, output := startSupervisedOwnedChild(t, "parent", policy)
		descendant := ownedDescendant(t, output)
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		report, err := supervisor.Shutdown(ctx, "descendant ownership")
		if len(report.Children) != 1 || !report.Children[0].Reaped {
			t.Fatalf("incomplete report: %+v", report)
		}
		assertDescendantShutdown(t, report.Children[0].ShutdownOutcome, err)
		assertOwnedProcessGone(t, descendant)
		if supervisor.Len() != 0 {
			t.Fatal("completed child retained")
		}
	})
}

func ownedDescendant(t *testing.T, output string) *os.Process {
	t.Helper()
	var pid int
	if _, err := fmt.Sscanf(output, "DESCENDANT:%d", &pid); err != nil || pid <= 0 {
		t.Fatalf("invalid owned descendant identity: %q %v", output, err)
	}
	descendant, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := descendant.Signal(syscall.Signal(0)); err == nil {
			if err := descendant.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				t.Error(err)
			}
		}
		assertOwnedProcessGone(t, descendant)
		if err := descendant.Release(); err != nil {
			t.Error(err)
		}
	})
	return descendant
}

func assertDescendantShutdown(t *testing.T, outcome process.ShutdownOutcome, err error) {
	t.Helper()
	if err == nil || outcome.Result == nil || !outcome.Result.Forced ||
		outcome.Result.ExitCode == nil || *outcome.Result.ExitCode != 0 {
		t.Fatalf("descendant escalation not reported independently of parent success: %+v %v", outcome, err)
	}
}

func assertOwnedProcessGone(t *testing.T, child *os.Process) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := child.Signal(syscall.Signal(0))
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("owned descendant %d retained: %v", child.Pid, ctx.Err())
		}
	}
}
