package worker

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/util"
	"github.com/kbukum/gokit/worker"
)

// Publication maps a worker event to an authorized audience pattern and a generated application message.
type Publication struct {
	Pattern string
	Message proto.Message
}

// Mapper is an application-owned projection to its proto contract. Normalize event errors before including their approved public fields.
type Mapper[O any] func(worker.Event[O]) (Publication, error)

// Forward runs in the caller's lifecycle until cancellation, input closure, or the first mapping/publication error. It never starts a detached goroutine, substitutes a success-shaped payload, or retries. When consuming TaskHandle.Events, the caller must cancel that task if forwarding exits early, releasing producer backpressure. Upstream pool-wide event retention is owned by the worker pool.
func Forward[O any](ctx context.Context, events <-chan worker.Event[O], publisher sse.Publisher, mapper Mapper[O]) error {
	if events == nil || util.IsNil(publisher) || mapper == nil {
		return apperrors.InvalidInput("bridge", "worker events, publisher, and proto mapper are required")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-events:
			if !ok {
				return nil
			}
			publication, err := mapper(event)
			if err != nil {
				return fmt.Errorf("mapping worker event: %w", err)
			}
			if err := publisher.Publish(ctx, publication.Pattern, publication.Message); err != nil {
				return fmt.Errorf("publishing worker event: %w", err)
			}
		}
	}
}
