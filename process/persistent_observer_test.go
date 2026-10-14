package process_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/kbukum/gokit/process"
)

type outputObserver struct {
	mu    sync.Mutex
	data  bytes.Buffer
	cause error
}

func (o *outputObserver) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.cause != nil {
		return 0, o.cause
	}
	return o.data.Write(data)
}

func TestPersistentObservesOutputAndOwnsSignals(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no SIGTERM contract")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := &outputObserver{}, &outputObserver{}
	cfg := process.DefaultPersistentConfig()
	cfg.Readiness, cfg.OutputMarker = process.ReadyOnOutput, "READY"
	cfg.Stdout, cfg.Stderr = stdout, stderr
	run, err := process.StartPersistent(t.Context(), process.Command{
		Binary: binary, Args: []string{"-test.run=^TestOwnedProcessChild$"},
		EnvPolicy: process.EnvEmpty,
		Env:       map[string]string{"GOKIT_OWNED_CHILD": "graceful", "GORACE": "atexit_sleep_ms=0"},
	}, cfg)
	if run != nil {
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
			defer cancel()
			if outcome, err := run.Process.Shutdown(ctx); err != nil || !outcome.Complete {
				t.Errorf("cleanup: %+v %v", outcome, err)
			}
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result, err := run.Process.Wait(ctx)
	if err != nil || !result.Success() {
		t.Fatalf("owned signal/wait: %+v %v", result, err)
	}
	if !bytes.Contains(stdout.data.Bytes(), []byte("READY")) || stderr.data.Len() == 0 {
		t.Fatal("live output observers missed child output")
	}
	if err := run.Process.Signal(syscall.SIGTERM); err == nil {
		t.Fatal("released identity accepted another signal")
	}
}

func TestPersistentObserverFailureRetainsCauseAndDrains(t *testing.T) {
	t.Parallel()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	cause := errors.New("observer capture overflow")
	cfg := process.DefaultPersistentConfig()
	cfg.Readiness, cfg.OutputMarker = process.ReadyOnOutput, "READY"
	cfg.Stdout = &outputObserver{cause: cause}
	run, err := process.StartPersistent(t.Context(), process.Command{
		Binary: binary, Args: []string{"-test.run=^TestOwnedProcessChild$"},
		EnvPolicy: process.EnvEmpty,
		Env:       map[string]string{"GOKIT_OWNED_CHILD": "graceful", "GORACE": "atexit_sleep_ms=0"},
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 2*time.Second)
	defer cancel()
	outcome, err := run.Process.Shutdown(ctx)
	if !outcome.Complete || !errors.Is(err, cause) {
		t.Fatalf("observer failure hid cause or prevented draining: %+v %v", outcome, err)
	}
}

func TestPersistentWaitExitSeparatesStatusFromCleanup(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no SIGTERM contract")
	}
	child := startOwnedChild(t, "late-failure")
	if err := child.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result, err := child.WaitExit(ctx)
	if err != nil || result == nil || result.ExitCodeOr(-1) != 7 {
		t.Fatalf("exit status was conflated with ownership failure: %+v %v", result, err)
	}
	if _, err := child.Wait(ctx); err == nil {
		t.Fatal("checked wait accepted a nonzero exit")
	}
}
