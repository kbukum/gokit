package worker_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"go.uber.org/goleak"
	"google.golang.org/protobuf/types/known/apipb"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/sse"
	sseworker "github.com/kbukum/gokit/sse/worker"
	"github.com/kbukum/gokit/worker"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestForwardFailureDoesNotStrandProducer(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"mapping", "publication", "cancellation"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				handler := worker.HandlerFunc[int, int](func(ctx context.Context, _ int, emit func(worker.Event[int])) error {
					for i := range 100 {
						if err := ctx.Err(); err != nil {
							return err
						}
						emit(worker.Event[int]{Type: worker.EventProgress, Data: i})
					}
					<-ctx.Done()
					return ctx.Err()
				})
				pool := worker.NewPool(handler, worker.PoolConfig{Name: "forward-owner", Size: 1, EventBuffer: 1, GracePeriod: 10 * time.Millisecond})
				handle, err := pool.Submit(t.Context(), 1)
				if err != nil {
					t.Fatal(err)
				}

				bus, err := sse.NewBus(sse.DefaultLimits())
				if err != nil {
					t.Fatal(err)
				}
				defer bus.Close()
				mapper := func(worker.Event[int]) (sseworker.Publication, error) {
					if mode == "mapping" {
						return sseworker.Publication{}, apperrors.InvalidInput("event", "mapping failed")
					}
					return sseworker.Publication{Pattern: "scope", Message: &apipb.Method{}}, nil
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if mode == "publication" {
					bus.Close()
				}
				if mode == "cancellation" {
					cancel()
				}
				if err := sseworker.Forward(ctx, handle.Events(), bus, mapper); err == nil {
					t.Fatal("expected forwarding failure")
				}
				synctest.Wait()
				handle.Cancel()
				synctest.Wait()
				select {
				case <-handle.Done():
				default:
					// Release the old defective implementation so a failed regression does not leak its test worker.
					for range handle.Events() {
					}
					t.Error("canceled producer remained blocked after forwarding stopped")
				}
				if _, err := handle.Result(); !errors.Is(err, context.Canceled) {
					t.Fatalf("task result: %v", err)
				}
				start := time.Now()
				if err := pool.Stop(t.Context()); err != nil {
					t.Fatal(err)
				}
				if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
					t.Fatalf("shutdown exceeded 10 ms: %s", elapsed)
				}
			})
		})
	}
}
