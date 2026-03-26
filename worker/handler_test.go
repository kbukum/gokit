package worker_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/worker"
)

func TestHandlerFunc(t *testing.T) {
	t.Parallel()

	var called bool
	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		called = true
		if task != "hello" {
			t.Fatalf("expected task 'hello', got %q", task)
		}
		emit(worker.PartialEvent("result"))
		return nil
	})

	var events []worker.Event[string]
	emit := func(e worker.Event[string]) { events = append(events, e) }

	err := h.Handle(context.Background(), "hello", emit)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("handler was not called")
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != worker.EventPartial {
		t.Fatalf("expected EventPartial, got %v", events[0].Type)
	}
}

func TestHandlerContextCancellation(t *testing.T) {
	t.Parallel()

	h := worker.HandlerFunc[string, string](func(
		ctx context.Context, task string, emit func(worker.Event[string]),
	) error {
		<-ctx.Done()
		return ctx.Err()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := h.Handle(ctx, "test", func(worker.Event[string]) {})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}
