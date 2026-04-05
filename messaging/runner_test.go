package messaging

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestAsRunner_Topic(t *testing.T) {
	sc := &stubConsumer{topic: "runner-topic"}
	handler := func(_ context.Context, _ Message) error { return nil }
	r := AsRunner(sc, handler)

	if got := r.Topic(); got != "runner-topic" {
		t.Fatalf("Topic() = %q, want %q", got, "runner-topic")
	}
}

func TestAsRunner_Close(t *testing.T) {
	var closeCalled atomic.Bool
	sc := &stubConsumer{
		topic: "close-topic",
		closeFn: func() error {
			closeCalled.Store(true)
			return nil
		},
	}
	handler := func(_ context.Context, _ Message) error { return nil }
	r := AsRunner(sc, handler)

	if err := r.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !closeCalled.Load() {
		t.Fatal("expected Close to delegate to underlying consumer")
	}
}

func TestAsRunner_Close_Error(t *testing.T) {
	wantErr := errors.New("close failed")
	sc := &stubConsumer{
		topic:   "close-err-topic",
		closeFn: func() error { return wantErr },
	}
	r := AsRunner(sc, func(_ context.Context, _ Message) error { return nil })

	if err := r.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("Close error = %v, want %v", err, wantErr)
	}
}

func TestAsRunner_Consume(t *testing.T) {
	var handlerCalled atomic.Bool
	handler := func(_ context.Context, _ Message) error {
		handlerCalled.Store(true)
		return nil
	}

	sc := &stubConsumer{
		topic: "consume-topic",
		consumeFn: func(ctx context.Context, h MessageHandler) error {
			// Call the handler to verify it was wired through
			if err := h(ctx, Message{Topic: "consume-topic"}); err != nil {
				return err
			}
			return nil
		},
	}

	r := AsRunner(sc, handler)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := r.Consume(ctx); err != nil {
		t.Fatalf("Consume returned error: %v", err)
	}

	if !handlerCalled.Load() {
		t.Fatal("expected handler to be called during Consume")
	}
}

func TestAsRunner_Consume_ContextCancellation(t *testing.T) {
	sc := &stubConsumer{topic: "cancel-topic"}
	r := AsRunner(sc, func(_ context.Context, _ Message) error { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := r.Consume(ctx)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("Consume error = %v, want context.Canceled", err)
	}
}
