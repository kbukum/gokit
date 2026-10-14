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

// unsyncedWriter is deliberately not concurrency-safe so the race detector proves serialized observer calls.
type unsyncedWriter struct {
	data  bytes.Buffer
	calls int
	cause error
}

func (w *unsyncedWriter) Write(data []byte) (int, error) {
	w.calls++
	if w.cause != nil {
		return 0, w.cause
	}
	return w.data.Write(data)
}

func shutdownGracefulChild(t *testing.T, cfg process.PersistentConfig) error {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Readiness, cfg.OutputMarker = process.ReadyOnOutput, "READY"
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
	if !outcome.Complete {
		t.Fatalf("shutdown did not complete: %+v %v", outcome, err)
	}
	return err
}

func TestPersistentSharedObserverIsSerialized(t *testing.T) {
	t.Parallel()
	shared := &unsyncedWriter{}
	cfg := process.DefaultPersistentConfig()
	cfg.Stdout, cfg.Stderr = shared, shared
	if err := shutdownGracefulChild(t, cfg); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(shared.data.Bytes(), []byte("READY")) || !bytes.Contains(shared.data.Bytes(), []byte("b")) {
		t.Fatal("shared observer missed combined output")
	}
}

func TestPersistentSharedObserverFailureStopsBothStreams(t *testing.T) {
	t.Parallel()
	cause := errors.New("shared observer failed")
	shared := &unsyncedWriter{cause: cause}
	cfg := process.DefaultPersistentConfig()
	cfg.Stdout, cfg.Stderr = shared, shared
	if err := shutdownGracefulChild(t, cfg); !errors.Is(err, cause) {
		t.Fatalf("shared observer failure lost its cause: %v", err)
	}
	if shared.calls != 1 {
		t.Fatalf("failed shared observer was called %d times", shared.calls)
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
