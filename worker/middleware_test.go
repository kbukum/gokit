package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/kbukum/gokit/worker"
)

func TestChain(t *testing.T) {
	t.Parallel()

	var order []string

	mwA := func(inner worker.Handler[string, string]) worker.Handler[string, string] {
		return worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
			order = append(order, "a-in")
			err := inner.Handle(ctx, task, emit)
			order = append(order, "a-out")
			return err
		})
	}

	mwB := func(inner worker.Handler[string, string]) worker.Handler[string, string] {
		return worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
			order = append(order, "b-in")
			err := inner.Handle(ctx, task, emit)
			order = append(order, "b-out")
			return err
		})
	}

	base := worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
		order = append(order, "handler")
		return nil
	})

	wrapped := worker.Chain(mwA, mwB)(base)
	_ = wrapped.Handle(context.Background(), "test", func(worker.Event[string]) {})

	expected := []string{"a-in", "b-in", "handler", "b-out", "a-out"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Fatalf("call %d: expected %q, got %q", i, v, order[i])
		}
	}
}

func TestWithTimeout(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
		<-ctx.Done()
		return ctx.Err()
	})

	wrapped := worker.WithTimeout[string, string](50 * time.Millisecond)(h)

	start := time.Now()
	err := wrapped.Handle(context.Background(), "test", func(worker.Event[string]) {})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

func TestWithRecovery(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
		panic("oops")
	})

	wrapped := worker.WithRecovery[string, string]()(h)

	err := wrapped.Handle(context.Background(), "test", func(worker.Event[string]) {})
	if err == nil {
		t.Fatal("expected error from recovered panic")
	}

	var panicErr *worker.PanicError
	if pe, ok := err.(*worker.PanicError); ok {
		panicErr = pe
	}
	if panicErr == nil {
		t.Fatalf("expected PanicError, got %T", err)
	}
}

func TestWithRecoveryErrorPanic(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(ctx context.Context, task string, emit func(worker.Event[string])) error {
		panic(context.DeadlineExceeded)
	})

	wrapped := worker.WithRecovery[string, string]()(h)

	err := wrapped.Handle(context.Background(), "test", func(worker.Event[string]) {})
	if err != context.DeadlineExceeded {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestPanicErrorString(t *testing.T) {
	t.Parallel()

	pe := &worker.PanicError{Value: "something went wrong"}
	s := pe.Error()
	if s != "worker: panic recovered" {
		t.Fatalf("expected 'worker: panic recovered', got %q", s)
	}
}
