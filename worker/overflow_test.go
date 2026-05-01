package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/worker"
)

func TestPoolOverflowRejectPoolWideCapacity(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	started := make(chan int, 2)
	h := worker.HandlerFunc[int, int](func(ctx context.Context, task int, emit func(worker.Event[int])) error {
		if task <= 2 {
			started <- task
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "reject", Size: 2, QueueSize: 1, Overflow: worker.OverflowReject})
	defer func() { _ = pool.Stop(context.Background()) }()
	defer close(block)

	for i := 1; i <= 2; i++ {
		if _, err := pool.Submit(context.Background(), i); err != nil {
			t.Fatalf("submit running task %d: %v", i, err)
		}
		waitStarted(t, started, 1)
	}
	if _, err := pool.Submit(context.Background(), 3); err != nil {
		t.Fatalf("submit queued task: %v", err)
	}
	if _, err := pool.Submit(context.Background(), 4); !errors.Is(err, worker.ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
}

func TestPoolOverflowDropOldestPoolWideFairness(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	var unblock sync.Once
	started := make(chan int, 2)
	h := worker.HandlerFunc[int, int](func(ctx context.Context, task int, emit func(worker.Event[int])) error {
		if task <= 2 {
			started <- task
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "drop", Size: 2, QueueSize: 2, Overflow: worker.OverflowDropOldest})
	defer func() { _ = pool.Stop(context.Background()) }()
	defer unblock.Do(func() { close(block) })

	for i := 1; i <= 2; i++ {
		if _, err := pool.Submit(context.Background(), i); err != nil {
			t.Fatalf("submit running task %d: %v", i, err)
		}
		waitStarted(t, started, 1)
	}
	dropped, err := pool.Submit(context.Background(), 3)
	if err != nil {
		t.Fatalf("submit queued task 3: %v", err)
	}
	kept4, err := pool.Submit(context.Background(), 4)
	if err != nil {
		t.Fatalf("submit queued task 4: %v", err)
	}
	kept5, err := pool.Submit(context.Background(), 5)
	if err != nil {
		t.Fatalf("submit replacement task 5: %v", err)
	}

	if _, err := dropped.Result(); !errors.Is(err, worker.ErrTaskDropped) {
		t.Fatalf("expected oldest queued task to be dropped, got %v", err)
	}
	unblock.Do(func() { close(block) })
	for name, h := range map[string]*worker.TaskHandle[int]{"kept4": kept4, "kept5": kept5} {
		if _, err := h.Result(); err != nil {
			t.Fatalf("%s failed: %v", name, err)
		}
	}
}

func TestPoolOverflowRejectImmediateWhenFull(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	started := make(chan int, 1)
	h := worker.HandlerFunc[int, int](func(ctx context.Context, task int, emit func(worker.Event[int])) error {
		if task == 1 {
			started <- task
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "reject-immediate", Size: 1, QueueSize: 1, Overflow: worker.OverflowReject})
	defer func() { _ = pool.Stop(context.Background()) }()

	if _, err := pool.Submit(context.Background(), 1); err != nil {
		t.Fatalf("submit running task: %v", err)
	}
	waitStarted(t, started, 1)
	if _, err := pool.Submit(context.Background(), 2); err != nil {
		t.Fatalf("submit queued task: %v", err)
	}
	start := time.Now()
	if _, err := pool.Submit(context.Background(), 3); !errors.Is(err, worker.ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("reject took too long: %v", elapsed)
	}
	close(block)
}

func TestPoolOverflowBlockWaits(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	started := make(chan int, 1)
	h := worker.HandlerFunc[int, int](func(ctx context.Context, task int, emit func(worker.Event[int])) error {
		if task == 1 {
			started <- task
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "block", Size: 1, QueueSize: 1, Overflow: worker.OverflowBlock})
	defer func() { _ = pool.Stop(context.Background()) }()

	if _, err := pool.Submit(context.Background(), 1); err != nil {
		t.Fatalf("submit running task: %v", err)
	}
	waitStarted(t, started, 1)
	if _, err := pool.Submit(context.Background(), 2); err != nil {
		t.Fatalf("submit queued task: %v", err)
	}

	submitted := make(chan error, 1)
	go func() {
		_, err := pool.Submit(context.Background(), 3)
		submitted <- err
	}()

	select {
	case err := <-submitted:
		t.Fatalf("submit should block, returned early with %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(block)
	select {
	case err := <-submitted:
		if err != nil {
			t.Fatalf("submit failed after queue drained: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("submit did not unblock")
	}
}

func TestPoolStopUnblocksBlockedSubmit(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	var unblock sync.Once
	started := make(chan int, 1)
	h := worker.HandlerFunc[int, int](func(ctx context.Context, task int, emit func(worker.Event[int])) error {
		if task == 1 {
			started <- task
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "stop-unblocks", Size: 1, QueueSize: 1, Overflow: worker.OverflowBlock, GracePeriod: time.Second})
	defer unblock.Do(func() { close(block) })

	if _, err := pool.Submit(context.Background(), 1); err != nil {
		t.Fatalf("submit running task: %v", err)
	}
	waitStarted(t, started, 1)
	if _, err := pool.Submit(context.Background(), 2); err != nil {
		t.Fatalf("submit queued task: %v", err)
	}

	submitted := make(chan error, 1)
	go func() {
		_, err := pool.Submit(context.Background(), 3)
		submitted <- err
	}()

	select {
	case err := <-submitted:
		t.Fatalf("submit should block before stop, returned early with %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- pool.Stop(context.Background())
	}()

	select {
	case err := <-submitted:
		if err == nil {
			t.Fatal("blocked submit unexpectedly succeeded during stop")
		}
	case <-time.After(time.Second):
		t.Fatal("blocked submit did not unblock when stop began")
	}

	unblock.Do(func() { close(block) })
	if err := <-stopDone; err != nil {
		t.Fatalf("stop failed: %v", err)
	}
}

func TestSupervisorAffinityFallsBackToSharedQueue(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	started := make(chan int, 1)
	h := worker.HandlerFunc[int, int](func(ctx context.Context, task int, emit func(worker.Event[int])) error {
		if task == 1 {
			started <- task
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{
		Name:      "supervisor-fallback",
		Size:      1,
		QueueSize: 2,
		Overflow:  worker.OverflowReject,
		Supervisor: &worker.SupervisorConfig{
			RestartPolicy: worker.RestartAlways,
		},
	})
	defer func() { _ = pool.Stop(context.Background()) }()
	defer close(block)

	if _, err := pool.Submit(context.Background(), 1); err != nil {
		t.Fatalf("submit running task: %v", err)
	}
	waitStarted(t, started, 1)
	if _, err := pool.Submit(context.Background(), 2); err != nil {
		t.Fatalf("submit first queued task: %v", err)
	}
	if _, err := pool.Submit(context.Background(), 3); err != nil {
		t.Fatalf("submit second queued task via shared fallback: %v", err)
	}
	if _, err := pool.Submit(context.Background(), 4); err != nil {
		t.Fatalf("submit third queued task via shared fallback: %v", err)
	}
	if _, err := pool.Submit(context.Background(), 5); !errors.Is(err, worker.ErrQueueFull) {
		t.Fatalf("expected pool-wide ErrQueueFull after shared queue fills, got %v", err)
	}
}

func waitStarted(t *testing.T, ch <-chan int, n int) {
	t.Helper()
	for range n {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatal("worker did not start task")
		}
	}
}
