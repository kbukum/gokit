package worker

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestUnreadEventsCompleteAndSignalOverflow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewPool(HandlerFunc[int, int](func(_ context.Context, task int, emit func(Event[int])) error {
			for range 1000 {
				emit(Event[int]{Type: EventProgress})
			}
			emit(Event[int]{Type: EventResult, Data: task})
			return nil
		}), PoolConfig{Size: 1, EventBuffer: 2})
		handle, err := pool.Submit(t.Context(), 42)
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case <-handle.Done():
		default:
			handle.Cancel()
			synctest.Wait()
			t.Error("unread progress events blocked completion")
		}
		result, err := handle.Result()
		if err != nil || result != 42 {
			t.Errorf("authoritative result = %d, %v", result, err)
		}
		if err := pool.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
		for name, events := range map[string]<-chan Event[int]{"task": handle.Events(), "pool": pool.Events()} {
			count, overflow := 0, 0
			for event := range events {
				count++
				if errors.Is(event.Error, ErrEventOverflow) {
					overflow++
				}
			}
			if count != 3 || overflow != 1 {
				t.Errorf("%s: got %d frames and %d overflow signals, want 2 data + 1 control", name, count, overflow)
			}
		}
	})
}

func TestTaskCallbacksReleased(t *testing.T) {
	t.Parallel()
	pool := NewPool(HandlerFunc[int, int](func(context.Context, int, func(Event[int])) error {
		return nil
	}), PoolConfig{Size: 1, EventBuffer: 2})
	for i := range 10000 {
		handle, err := pool.Submit(t.Context(), i)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := handle.Result(); err != nil {
			t.Fatal(err)
		}
		if got := pool.Stats().CancellationCallbacks; got != 0 {
			t.Fatalf("submission %d retained %d callbacks", i, got)
		}
	}
	if err := pool.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}
