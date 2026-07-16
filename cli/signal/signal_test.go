package signal_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	clisignal "github.com/kbukum/gokit/cli/signal"
)

func signalSelf(t *testing.T) {
	t.Helper()
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find process: %v", err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal self: %v", err)
	}
}

func TestOnInterruptCancelsOnSignal(t *testing.T) {
	ctx, stop := clisignal.OnInterrupt(context.Background())
	defer stop()

	if err := ctx.Err(); err != nil {
		t.Fatalf("context must start live, got %v", err)
	}

	signalSelf(t)

	select {
	case <-ctx.Done():
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Errorf("err = %v, want Canceled", ctx.Err())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("context was not canceled after interrupt")
	}
}

func TestNotifyContextStopReleasesHandler(t *testing.T) {
	ctx, stop := clisignal.NotifyContext(context.Background(), os.Interrupt)
	stop()
	// stop cancels the returned context and releases the signal handler.
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
	ctx, stop := clisignal.NotifyContext(parent, os.Interrupt)
	defer stop()

	cancelParent()
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("child context must follow parent cancellation")
	}
}

func TestInterruptSignalsIncludeInterrupt(t *testing.T) {
	t.Parallel()
	sigs := clisignal.InterruptSignals()
	found := false
	for _, s := range sigs {
		if s == os.Interrupt {
			found = true
		}
	}
	if !found {
		t.Errorf("InterruptSignals = %v, want os.Interrupt", sigs)
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
