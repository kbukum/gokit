package middleware

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/messaging"
)

func TestDedupHandler_DuplicateSkipped(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	wrapped := DedupHandler(handler, DedupConfig{
		KeyFunc: func(msg messaging.Message) string { return msg.Key },
	})

	ctx := context.Background()
	msg := messaging.Message{Key: "dup-1"}
	_ = wrapped(ctx, msg)
	_ = wrapped(ctx, msg)
	_ = wrapped(ctx, msg)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}

func TestDedupHandler_UniqueMessagesPassed(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	wrapped := DedupHandler(handler, DedupConfig{
		KeyFunc: func(msg messaging.Message) string { return msg.Key },
	})

	ctx := context.Background()
	_ = wrapped(ctx, messaging.Message{Key: "a"})
	_ = wrapped(ctx, messaging.Message{Key: "b"})
	_ = wrapped(ctx, messaging.Message{Key: "c"})

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestDedupHandler_TTLExpiry(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	cfg := DedupConfig{
		KeyFunc: func(msg messaging.Message) string { return msg.Key },
		TTL:     50 * time.Millisecond,
	}
	wrapped := DedupHandler(handler, cfg)

	ctx := context.Background()
	msg := messaging.Message{Key: "ttl-1"}

	_ = wrapped(ctx, msg)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}

	time.Sleep(100 * time.Millisecond)

	_ = wrapped(ctx, msg)
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("calls after TTL = %d, want 2", got)
	}
}

func TestDedupHandler_WindowEviction(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	cfg := DedupConfig{
		KeyFunc:    func(msg messaging.Message) string { return msg.Key },
		WindowSize: 3,
		TTL:        time.Hour,
	}
	wrapped := DedupHandler(handler, cfg)

	ctx := context.Background()
	// Fill the window: a, b, c
	_ = wrapped(ctx, messaging.Message{Key: "a"})
	_ = wrapped(ctx, messaging.Message{Key: "b"})
	_ = wrapped(ctx, messaging.Message{Key: "c"})

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}

	// Adding d should evict a (oldest).
	_ = wrapped(ctx, messaging.Message{Key: "d"})

	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Fatalf("calls = %d, want 4", got)
	}

	// a was evicted, so it should be treated as new.
	_ = wrapped(ctx, messaging.Message{Key: "a"})
	if got := atomic.LoadInt32(&calls); got != 5 {
		t.Errorf("calls = %d, want 5 (a was evicted, should pass)", got)
	}

	// b should still be in the cache (b, c were retained; d and a are newest).
	// Actually: after adding d, window is [d, c, b] (a evicted).
	// After adding a, window is [a, d, c] (b evicted).
	// So b should also be treated as new now.
	_ = wrapped(ctx, messaging.Message{Key: "b"})
	if got := atomic.LoadInt32(&calls); got != 6 {
		t.Errorf("calls = %d, want 6 (b was evicted, should pass)", got)
	}
}

func TestDedupHandler_EmptyKeyAlwaysProcessed(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	wrapped := DedupHandler(handler, DedupConfig{
		KeyFunc: func(msg messaging.Message) string { return msg.Key },
	})

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = wrapped(ctx, messaging.Message{Key: ""})
	}

	if got := atomic.LoadInt32(&calls); got != 5 {
		t.Errorf("calls = %d, want 5 (empty key should always process)", got)
	}
}

func TestDedupHandler_DefaultKeyFunc(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := func(_ context.Context, _ messaging.Message) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}

	wrapped := DedupHandler(handler, DedupConfig{})

	ctx := context.Background()
	msg := messaging.Message{Headers: map[string]string{"message-id": "id-1"}}
	_ = wrapped(ctx, msg)
	_ = wrapped(ctx, msg)

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}
