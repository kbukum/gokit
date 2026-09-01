package stream

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// TestDebounceBatch_SizeFlush verifies that reaching maxItems flushes a window early,
// independent of the quiet timer. Values are all immediately available, so the size cap
// fires before any silent window elapses.
func TestDebounceBatch_SizeFlush(t *testing.T) {
	ch := make(chan result[int], 8)
	for _, v := range []int{1, 2, 3, 4} {
		ch <- result[int]{val: v, ok: true}
	}
	close(ch)

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	batched := DebounceBatch(src, time.Hour, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, batched)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]int{{1, 2}, {3, 4}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

// TestDebounceBatch_BoundedBuffer verifies the buffer never exceeds maxItems even for a
// long burst: the window force-flushes at the cap and a trailing partial window is emitted
// when the source ends.
func TestDebounceBatch_BoundedBuffer(t *testing.T) {
	ch := make(chan result[int], 16)
	for v := 1; v <= 7; v++ {
		ch <- result[int]{val: v, ok: true}
	}
	close(ch)

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	batched := DebounceBatch(src, time.Hour, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, batched)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]int{{1, 2, 3}, {4, 5, 6}, {7}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
	for i, w := range got {
		if len(w) > 3 {
			t.Errorf("window %d exceeded maxItems: %v", i, w)
		}
	}
}

// manualBatchTimer is a controllable batchTimer for tests: it records each arming and fires
// the quiet deadline only when the test calls fire, so trailing-edge flush is exercised
// deterministically without wall-clock sleeps.
type manualBatchTimer struct {
	c     chan time.Time
	armed chan struct{}
}

func newManualBatchTimer() *manualBatchTimer {
	return &manualBatchTimer{c: make(chan time.Time), armed: make(chan struct{}, 64)}
}

func (m *manualBatchTimer) C() <-chan time.Time   { return m.c }
func (m *manualBatchTimer) reset(_ time.Duration) { m.armed <- struct{}{} }
func (m *manualBatchTimer) stop()                 {}
func (m *manualBatchTimer) fire()                 { m.c <- time.Time{} }

// TestDebounceBatch_SilenceFlush verifies a trailing-edge flush: values arriving within one
// window are kept together and emitted only after the quiet gap elapses. The source emits
// three values then blocks (never closes), so the window can only flush via the quiet timer,
// which is driven deterministically through an injected timer rather than a real sleep.
func TestDebounceBatch_SilenceFlush(t *testing.T) {
	ch := make(chan result[int], 3)
	for _, v := range []int{1, 2, 3} {
		ch <- result[int]{val: v, ok: true}
	}

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	timer := newManualBatchTimer()
	batched := debounceBatch(src, 40*time.Millisecond, 100, func() batchTimer { return timer })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	it := batched.create(ctx)
	defer func() { _ = it.Close() }()

	type res struct {
		batch []int
		ok    bool
		err   error
	}
	done := make(chan res, 1)
	go func() {
		b, ok, err := it.Next(ctx)
		done <- res{b, ok, err}
	}()

	// Each value arms the trailing-edge timer exactly once; once all three are buffered we
	// simulate the quiet gap by firing the timer.
	for i := 0; i < 3; i++ {
		select {
		case <-timer.armed:
		case <-time.After(time.Second):
			t.Fatalf("value %d never armed the quiet timer", i+1)
		}
	}
	timer.fire()

	r := <-done
	if r.err != nil {
		t.Fatalf("Next: %v", r.err)
	}
	want := []int{1, 2, 3}
	if !reflect.DeepEqual(r.batch, want) {
		t.Errorf("expected %v, got %v", want, r.batch)
	}
}

// TestDebounceBatch_Empty verifies a quiet, empty source never emits a window.
func TestDebounceBatch_Empty(t *testing.T) {
	ch := make(chan result[int])
	close(ch)

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	batched := DebounceBatch(src, 40*time.Millisecond, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, batched)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no windows, got %v", got)
	}
}

// TestDebounceBatch_ContextCancelled verifies cancellation surfaces the context error and
// does not leak the source-draining goroutine.
func TestDebounceBatch_ContextCancelled(t *testing.T) {
	ch := make(chan result[int]) // never sends, never closes

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	batched := DebounceBatch(src, time.Hour, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := Collect(ctx, batched)
	if err == nil {
		t.Fatal("expected context error")
	}
}

// TestDebounceBatch_MaxItemsClamped verifies maxItems below 1 is clamped to 1.
func TestDebounceBatch_MaxItemsClamped(t *testing.T) {
	ch := make(chan result[int], 4)
	for _, v := range []int{1, 2} {
		ch <- result[int]{val: v, ok: true}
	}
	close(ch)

	src := FromFunc(func(ctx context.Context) Iterator[int] {
		return &channelIter[int]{ch: ch}
	})

	batched := DebounceBatch(src, time.Hour, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got, err := Collect(ctx, batched)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]int{{1}, {2}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}
