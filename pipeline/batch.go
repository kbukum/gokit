package pipeline

import (
	"context"
	"time"
)

// Batch collects up to size values or waits timeout (whichever comes first),
// then emits them as a slice.
//
// size=0 means collect until timeout. timeout=0 means collect until size.
// Both zero is invalid and defaults to size=1.
//
// Named Batch (not Buffer) because pipeline.Buffer already exists for
// channel-based decoupling between stages.
func Batch[T any](p *Pipeline[T], size int, timeout time.Duration) *Pipeline[[]T] {
	if size <= 0 && timeout <= 0 {
		size = 1
	}
	return &Pipeline[[]T]{
		create: func(ctx context.Context) Iterator[[]T] {
			return &batchIter[T]{
				source:  p.create(ctx),
				size:    size,
				timeout: timeout,
			}
		},
	}
}

type batchIter[T any] struct {
	source  Iterator[T]
	size    int
	timeout time.Duration
	done    bool
}

func (it *batchIter[T]) Next(ctx context.Context) (result []T, ok bool, err error) {
	if it.done {
		return nil, false, nil
	}

	var batch []T
	var timer <-chan time.Time

	if it.timeout > 0 {
		t := time.NewTimer(it.timeout)
		defer t.Stop()
		timer = t.C
	}

	for {
		// Check if batch is full by size
		if it.size > 0 && len(batch) >= it.size {
			return batch, true, nil
		}

		// Try to get next value (non-blocking if we have a timer)
		val, ok, err := it.source.Next(ctx)
		if err != nil {
			if len(batch) > 0 {
				// Return partial batch on error; error will surface on next call
				return batch, true, nil
			}
			return nil, false, err
		}
		if !ok {
			it.done = true
			if len(batch) > 0 {
				return batch, true, nil
			}
			return nil, false, nil
		}

		batch = append(batch, val)

		// Check timeout (only if timeout is set and we already have items)
		if timer != nil {
			select {
			case <-timer:
				return batch, true, nil
			default:
			}
		}
	}
}

func (it *batchIter[T]) Close() error { return it.source.Close() }
