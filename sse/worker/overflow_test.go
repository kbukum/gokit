package worker_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/types/known/apipb"

	"github.com/kbukum/gokit/sse"
	sseworker "github.com/kbukum/gokit/sse/worker"
	"github.com/kbukum/gokit/worker"
)

func TestForwardReportsPoolOverflow(t *testing.T) {
	t.Parallel()
	pool := worker.NewPool(worker.HandlerFunc[int, int](func(_ context.Context, _ int, emit func(worker.Event[int])) error {
		for range 1000 {
			emit(worker.Event[int]{Type: worker.EventProgress})
		}
		return nil
	}), worker.PoolConfig{Size: 1, EventBuffer: 2})
	t.Cleanup(func() {
		if err := pool.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	handle, err := pool.Submit(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handle.Result(); err != nil {
		t.Fatal(err)
	}
	bus, err := sse.NewBus(sse.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer bus.Close()
	err = sseworker.Forward(t.Context(), handle.Events(), bus, func(worker.Event[int]) (sseworker.Publication, error) {
		return sseworker.Publication{Pattern: "scope", Message: &apipb.Method{}}, nil
	})
	if !errors.Is(err, worker.ErrEventOverflow) {
		t.Fatalf("lost delivery reported as %v", err)
	}
}
