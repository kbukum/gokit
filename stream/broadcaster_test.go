package stream_test

import (
	"context"
	"testing"
	"time"

	"github.com/kbukum/gokit/stream"
)

// closeWait bounds how long a test waits for a subscriber channel to close, so a
// regression that fails to close it reports a failure instead of stalling CI.
const closeWait = 2 * time.Second

// drainUntilClosed reads from ch until it is closed, failing if that does not
// happen within closeWait.
func drainUntilClosed[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	deadline := time.After(closeWait)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("channel did not close within timeout")
		}
	}
}

// expectClosed asserts ch is closed (yields no value) within closeWait.
func expectClosed[T any](t *testing.T, ch <-chan T, name string) {
	t.Helper()
	select {
	case v, ok := <-ch:
		if ok {
			t.Fatalf("%s should be closed, got %v", name, v)
		}
	case <-time.After(closeWait):
		t.Fatalf("%s did not close within timeout", name)
	}
}

func TestBroadcasterDeliversToAllSubscribers(t *testing.T) {
	t.Parallel()

	b := stream.NewBroadcaster[int]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s1 := b.Subscribe(ctx)
	s2 := b.Subscribe(ctx)
	if b.SubscriberCount() != 2 {
		t.Fatalf("subscriber count = %d, want 2", b.SubscriberCount())
	}

	b.Broadcast(7)
	if v := <-s1; v != 7 {
		t.Fatalf("s1 = %d, want 7", v)
	}
	if v := <-s2; v != 7 {
		t.Fatalf("s2 = %d, want 7", v)
	}
}

func TestBroadcasterDefaultBufferAndOption(t *testing.T) {
	t.Parallel()

	if b := stream.NewBroadcaster[int](); b.Buffer() != stream.DefaultBroadcastBuffer {
		t.Fatalf("default buffer = %d, want %d", b.Buffer(), stream.DefaultBroadcastBuffer)
	}
	if b := stream.NewBroadcaster[int](stream.WithBroadcastBuffer(8)); b.Buffer() != 8 {
		t.Fatalf("buffer = %d, want 8", b.Buffer())
	}
	// Non-positive buffer is clamped to 1.
	if b := stream.NewBroadcaster[int](stream.WithBroadcastBuffer(0)); b.Buffer() != 1 {
		t.Fatalf("clamped buffer = %d, want 1", b.Buffer())
	}
}

func TestBroadcasterDropsOverflowWithoutBlocking(t *testing.T) {
	t.Parallel()

	b := stream.NewBroadcaster[int](stream.WithBroadcastBuffer(1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := b.Subscribe(ctx)

	// The subscriber never drains; extra events must be dropped, not block.
	b.Broadcast(1)
	b.Broadcast(2)
	b.Broadcast(3)

	if v := <-sub; v != 1 {
		t.Fatalf("first delivered = %d, want 1 (overflow dropped)", v)
	}
	select {
	case v := <-sub:
		t.Fatalf("expected empty buffer after drain, got %d", v)
	default:
	}
}

func TestBroadcasterSubscribeCanceledContext(t *testing.T) {
	t.Parallel()

	b := stream.NewBroadcaster[int]()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sub := b.Subscribe(ctx)
	// A subscription made with an already-canceled context must not be
	// registered and must yield an already-closed channel.
	if n := b.SubscriberCount(); n != 0 {
		t.Fatalf("subscriber count = %d, want 0 for canceled-context subscribe", n)
	}
	b.Broadcast(1)
	expectClosed(t, sub, "canceled-context subscription")
}

func TestBroadcasterChannelClosesOnCancel(t *testing.T) {
	t.Parallel()

	b := stream.NewBroadcaster[int]()
	ctx, cancel := context.WithCancel(context.Background())
	sub := b.Subscribe(ctx)

	cancel()
	// Ranging over the channel must terminate once cancellation prunes the sub.
	drainUntilClosed(t, sub)
}

func TestBroadcasterCloseClosesAllSubscribers(t *testing.T) {
	t.Parallel()

	b := stream.NewBroadcaster[int]()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s1 := b.Subscribe(ctx)
	s2 := b.Subscribe(ctx)

	b.Close()
	expectClosed(t, s1, "s1")
	expectClosed(t, s2, "s2")

	// Close is idempotent and Broadcast/Subscribe are safe post-close.
	b.Close()
	b.Broadcast(1)
	sub := b.Subscribe(ctx)
	expectClosed(t, sub, "post-close subscription")
}
