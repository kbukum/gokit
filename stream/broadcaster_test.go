package stream_test

import (
	"context"
	"sync"
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

func TestBroadcasterDroppedCountTracksOverflow(t *testing.T) {
	t.Parallel()

	b := stream.NewBroadcaster[int](stream.WithBroadcastBuffer(1))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := b.Subscribe(ctx)

	if got := b.DroppedCount(); got != 0 {
		t.Fatalf("fresh DroppedCount = %d, want 0", got)
	}

	// First event fills the buffer (no drop); the next two overflow it.
	b.Broadcast(1)
	if got := b.DroppedCount(); got != 0 {
		t.Fatalf("DroppedCount after buffered delivery = %d, want 0", got)
	}
	b.Broadcast(2)
	b.Broadcast(3)
	if got := b.DroppedCount(); got != 2 {
		t.Fatalf("DroppedCount after overflow = %d, want 2", got)
	}

	// The buffered event still arrives: drop observability leaves delivery intact.
	if v := <-sub; v != 1 {
		t.Fatalf("first delivered = %d, want 1", v)
	}
}

func TestBroadcasterDroppedCountFreshAndNormalDelivery(t *testing.T) {
	t.Parallel()

	b := stream.NewBroadcaster[int](stream.WithBroadcastBuffer(4))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := b.Subscribe(ctx)

	for i := 1; i <= 4; i++ {
		b.Broadcast(i)
	}
	for i := 1; i <= 4; i++ {
		if v := <-sub; v != i {
			t.Fatalf("delivered = %d, want %d", v, i)
		}
	}
	if got := b.DroppedCount(); got != 0 {
		t.Fatalf("DroppedCount after normal delivery = %d, want 0", got)
	}
}

func TestBroadcasterOnDropFiresExactlyOnOverflow(t *testing.T) {
	t.Parallel()

	var drops int
	b := stream.NewBroadcaster[int](
		stream.WithBroadcastBuffer(1),
		stream.WithBroadcastOnDrop(func() { drops++ }),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	healthy := b.Subscribe(ctx)
	slow := b.Subscribe(ctx)

	// healthy drains after each send; slow never drains, so it overflows.
	b.Broadcast(1)
	if v := <-healthy; v != 1 {
		t.Fatalf("healthy first = %d, want 1", v)
	}
	if drops != 0 {
		t.Fatalf("OnDrop fired on normal delivery: drops = %d", drops)
	}

	b.Broadcast(2) // slow buffer full (holds 1) -> drop; healthy gets 2
	if v := <-healthy; v != 2 {
		t.Fatalf("healthy second = %d, want 2", v)
	}
	b.Broadcast(3) // slow still full -> drop; healthy gets 3
	if v := <-healthy; v != 3 {
		t.Fatalf("healthy third = %d, want 3", v)
	}

	if drops != 2 {
		t.Fatalf("OnDrop fired %d times, want 2", drops)
	}
	if got := b.DroppedCount(); got != 2 {
		t.Fatalf("DroppedCount = %d, want 2", got)
	}
	// slow still holds its one buffered event.
	if v := <-slow; v != 1 {
		t.Fatalf("slow buffered = %d, want 1", v)
	}
}

func TestBroadcasterDropAfterCloseIsNoOp(t *testing.T) {
	t.Parallel()

	var drops int
	b := stream.NewBroadcaster[int](
		stream.WithBroadcastBuffer(1),
		stream.WithBroadcastOnDrop(func() { drops++ }),
	)
	b.Close()

	b.Broadcast(1)
	b.Broadcast(2)
	if drops != 0 {
		t.Fatalf("OnDrop fired after Close: drops = %d", drops)
	}
	if got := b.DroppedCount(); got != 0 {
		t.Fatalf("DroppedCount after Close no-op = %d, want 0", got)
	}
}

func TestBroadcasterOnDropUnderConcurrentBroadcast(t *testing.T) {
	t.Parallel()

	const senders = 8
	const perSender = 50
	var mu sync.Mutex
	hookDrops := 0

	b := stream.NewBroadcaster[int](
		stream.WithBroadcastBuffer(1),
		stream.WithBroadcastOnDrop(func() {
			mu.Lock()
			hookDrops++
			mu.Unlock()
		}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = b.Subscribe(ctx) // never drained: every overflow drops

	var wg sync.WaitGroup
	for s := 0; s < senders; s++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perSender; i++ {
				b.Broadcast(i)
			}
		}()
	}
	wg.Wait()

	// One event fills the buffer; every remaining send drops.
	const total = senders * perSender
	wantDrops := uint64(total - 1)
	if got := b.DroppedCount(); got != wantDrops {
		t.Fatalf("DroppedCount = %d, want %d", got, wantDrops)
	}
	mu.Lock()
	got := hookDrops
	mu.Unlock()
	if uint64(got) != wantDrops {
		t.Fatalf("OnDrop fired %d times, want %d", got, wantDrops)
	}
}

func TestBroadcasterZeroValueUsable(t *testing.T) {
	t.Parallel()

	// A zero-value Broadcaster must lazily initialize and never panic: Subscribe,
	// Broadcast, and Close all work with the default buffer.
	var b stream.Broadcaster[int]
	if got := b.Buffer(); got != stream.DefaultBroadcastBuffer {
		t.Fatalf("zero-value buffer = %d, want %d", got, stream.DefaultBroadcastBuffer)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := b.Subscribe(ctx)
	b.Broadcast(9)
	if v := <-sub; v != 9 {
		t.Fatalf("zero-value delivery = %d, want 9", v)
	}

	b.Close() // must not panic on a lazily-initialized done channel
	expectClosed(t, sub, "zero-value subscription")
}

func TestBroadcasterZeroValueCloseWithoutSubscribers(t *testing.T) {
	t.Parallel()

	// Close on a never-used zero value must not panic on close(nil) and stays
	// idempotent.
	var b stream.Broadcaster[int]
	b.Close()
	b.Close()
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
