package worker_test

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/worker"
)

func TestTaskCancelMidExecution(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})

	h := worker.HandlerFunc[int, int](func(
		ctx context.Context, task int, emit func(worker.Event[int]),
	) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})

	pool := worker.NewPool(h, worker.PoolConfig{Name: "cancel-mid", Size: 1})
	defer func() { _ = pool.Stop(context.Background()) }()

	handle, err := pool.Submit(context.Background(), 1)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	<-started
	handle.Cancel()

	var gotError bool
	for e := range handle.Events() {
		if e.Type == worker.EventError {
			gotError = true
		}
	}

	_, herr := handle.Result()
	if herr == nil {
		t.Fatal("expected error from canceled task")
	}
	if !gotError {
		t.Fatal("expected error event from canceled task")
	}
}
