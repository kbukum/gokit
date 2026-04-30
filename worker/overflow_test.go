package worker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kbukum/gokit/worker"
)

func TestPoolOverflowReject(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	started := make(chan struct{}, 1)
	h := worker.HandlerFunc[int, int](func(ctx context.Context, task int, emit func(worker.Event[int])) error {
		if task == 1 {
			started <- struct{}{}
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "reject", Size: 1, QueueSize: 1, Overflow: worker.Reject})
	defer func() { _ = pool.Stop(context.Background()) }()

	if _, err := pool.Submit(context.Background(), 1); err != nil {
		t.Fatalf("submit running task: %v", err)
	}
	<-started
	if _, err := pool.Submit(context.Background(), 2); err != nil {
		t.Fatalf("submit queued task: %v", err)
	}
	if _, err := pool.Submit(context.Background(), 3); !errors.Is(err, worker.ErrQueueFull) {
		t.Fatalf("expected ErrQueueFull, got %v", err)
	}
	close(block)
}

func TestPoolOverflowDropOldest(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	started := make(chan struct{}, 1)
	h := worker.HandlerFunc[int, int](func(ctx context.Context, task int, emit func(worker.Event[int])) error {
		if task == 1 {
			started <- struct{}{}
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "drop", Size: 1, QueueSize: 1, Overflow: worker.DropOldest})
	defer func() { _ = pool.Stop(context.Background()) }()

	if _, err := pool.Submit(context.Background(), 1); err != nil {
		t.Fatalf("submit running task: %v", err)
	}
	<-started
	dropped, err := pool.Submit(context.Background(), 2)
	if err != nil {
		t.Fatalf("submit queued task: %v", err)
	}
	kept, err := pool.Submit(context.Background(), 3)
	if err != nil {
		t.Fatalf("submit replacement task: %v", err)
	}

	if _, err := dropped.Result(); !errors.Is(err, worker.ErrTaskDropped) {
		t.Fatalf("expected dropped task error, got %v", err)
	}
	close(block)
	if _, err := kept.Result(); err != nil {
		t.Fatalf("replacement task failed: %v", err)
	}
}

func TestPoolOverflowBlock(t *testing.T) {
	t.Parallel()

	block := make(chan struct{})
	started := make(chan struct{}, 1)
	h := worker.HandlerFunc[int, int](func(ctx context.Context, task int, emit func(worker.Event[int])) error {
		if task == 1 {
			started <- struct{}{}
			<-block
		}
		return nil
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "block", Size: 1, QueueSize: 1, Overflow: worker.Block})
	defer func() { _ = pool.Stop(context.Background()) }()

	if _, err := pool.Submit(context.Background(), 1); err != nil {
		t.Fatalf("submit running task: %v", err)
	}
	<-started
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
