package worker

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestRejectedSubmissionsReleaseCallbacks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		pool := NewPool(HandlerFunc[int, int](func(ctx context.Context, task int, _ func(Event[int])) error {
			if task == 1 {
				close(started)
			}
			<-ctx.Done()
			return ctx.Err()
		}), PoolConfig{Size: 1, QueueSize: 1, Overflow: OverflowReject, EventBuffer: 2, GracePeriod: time.Millisecond})
		first, err := pool.Submit(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		<-started
		second, err := pool.Submit(t.Context(), 2)
		if err != nil {
			t.Fatal(err)
		}
		for range 10000 {
			if _, err := pool.Submit(t.Context(), 3); !errors.Is(err, ErrQueueFull) {
				t.Fatalf("rejection: %v", err)
			}
			if got := pool.Stats().CancellationCallbacks; got != 2 {
				t.Fatalf("rejected submission retained callback: %d", got)
			}
		}
		if err := pool.Quiesce(); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Submit(t.Context(), 4); err == nil {
			t.Fatal("quiesced pool accepted work")
		}
		if err := pool.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
		for _, handle := range []*TaskHandle[int]{first, second} {
			if _, err := handle.Result(); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled result: %v", err)
			}
		}
		if stats := pool.Stats(); stats.CancellationCallbacks != 0 || stats.Active != 0 || stats.Queued != 0 {
			t.Fatalf("shutdown retained resources: %+v", stats)
		}
	})
}

func TestEventQueueBoundaryAndReservedOverflow(t *testing.T) {
	t.Parallel()
	queue := newEventQueue[int](2)
	queue.emit(Event[int]{Data: 1})
	queue.emit(Event[int]{Data: 2})
	if len(queue.events) != 2 || queue.closed {
		t.Fatal("queue overflowed before the boundary")
	}
	queue.emit(Event[int]{Data: 3})
	if len(queue.events) != 3 || !queue.closed {
		t.Fatal("one-over-limit did not close with a reserved control")
	}
	if (<-queue.events).Data != 1 || (<-queue.events).Data != 2 || !errors.Is((<-queue.events).Error, ErrEventOverflow) {
		t.Fatal("overflow did not preserve queued prefix plus explicit failure")
	}
	queue.emit(Event[int]{Data: 4})
	if _, ok := <-queue.events; ok {
		t.Fatal("closed event stream accepted another event")
	}
}
