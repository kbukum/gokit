package worker

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestMapReduceReusablePoolDoesNotWaitForPoolStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pool := NewPool(HandlerFunc[int, int](func(_ context.Context, task int, emit func(Event[int])) error {
			emit(Event[int]{Type: EventResult, Data: task})
			return nil
		}), PoolConfig{Size: 1})
		handler := NewMapReduce(MapReduceConfig[int, int, int]{
			Pool:    pool,
			Split:   func(input int) []int { return []int{input, input + 1} },
			Combine: func(results []int) (int, error) { return results[0] + results[1], nil },
		})
		done := make(chan error, 1)
		go func() {
			done <- handler.Handle(t.Context(), 2, func(Event[int]) {})
		}()
		synctest.Wait()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		default:
			t.Error("map-reduce retained its forwarding goroutine until shared pool shutdown")
		}
		if err := pool.Stop(t.Context()); err != nil {
			t.Fatal(err)
		}
	})
}
