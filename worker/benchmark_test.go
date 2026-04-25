package worker

import (
	"context"
	"testing"
)

func BenchmarkSchedulerDispatchRoundRobin(b *testing.B) {
	d := newDispatcher(RoundRobin)
	stats := make([]workerStats, 16)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.next(stats)
	}
}

func BenchmarkPoolSubmitHotPath(b *testing.B) {
	h := HandlerFunc[int, int](func(ctx context.Context, task int, emit func(Event[int])) error {
		emit(PartialEvent(task + 1))
		return nil
	})
	pool := NewPool(h, PoolConfig{Name: "bench", Size: 4})
	defer pool.Stop(context.Background())

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := pool.Submit(ctx, i); err != nil {
			b.Fatalf("submit failed: %v", err)
		}
	}
}
