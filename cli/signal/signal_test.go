package signal_test

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	clisignal "github.com/kbukum/gokit/cli/signal"
)

func TestOnInterruptCancelsOnSignal(t *testing.T) {
	ctx, stop := clisignal.OnInterrupt(context.Background())
	defer stop()

	if err := ctx.Err(); err != nil {
		t.Fatalf("context must start live, got %v", err)
	}

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill: %v", err)
	}

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Errorf("err = %v, want Canceled", ctx.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("context was not canceled after SIGTERM")
	}
}

func TestNotifyContextStopReleasesHandler(t *testing.T) {
	ctx, stop := clisignal.NotifyContext(context.Background(), syscall.SIGUSR1)
	stop()
	// After stop, canceling the parent still propagates; the handler is gone.
	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Errorf("err = %v, want Canceled", ctx.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("context must be canceled after stop")
	}
}

func TestNotifyContextFollowsParentCancellation(t *testing.T) {
	t.Parallel()
	parent, cancelParent := context.WithCancel(context.Background())
	ctx, stop := clisignal.NotifyContext(parent, syscall.SIGUSR2)
	defer stop()

	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("child context must follow parent cancelation")
	}
}

func TestInterruptSignalsIncludeSIGINTAndSIGTERM(t *testing.T) {
	t.Parallel()
	sigs := clisignal.InterruptSignals()
	has := map[string]bool{}
	for _, s := range sigs {
		has[s.String()] = true
	}
	if !has[syscall.SIGINT.String()] || !has[syscall.SIGTERM.String()] {
		t.Errorf("InterruptSignals = %v, want SIGINT and SIGTERM", sigs)
	}
}

func TestInterruptSignalsReturnsFreshSlice(t *testing.T) {
	t.Parallel()
	first := clisignal.InterruptSignals()
	first[0] = nil
	if clisignal.InterruptSignals()[0] == nil {
		t.Error("InterruptSignals must return a fresh slice, not shared state")
	}
}
