package stream

import (
	"context"
	"time"
)

// DebounceBatch accumulates values while they keep arriving, emitting the collected batch
// once quiet elapses with no new value (trailing-edge: the timer resets on every value),
// or early once maxItems have accumulated.
//
// Unlike Debounce, which keeps only the latest value seen during a burst, DebounceBatch
// keeps every value in arrival order. It also differs from Batch, whose timer runs from the
// first value (leading-edge); here the window is trailing-edge.
//
// maxItems (clamped to at least 1) bounds the buffer: because the timer resets on every
// value, a sustained arrival rate faster than quiet would otherwise never reach a silent
// window and grow the buffer without bound. Reaching maxItems emits the accumulated window
// immediately and starts a fresh one.
func DebounceBatch[T any](p *Pipeline[T], quiet time.Duration, maxItems int) *Pipeline[[]T] {
	return debounceBatch(p, quiet, maxItems, newRealBatchTimer)
}

// debounceBatch is the DebounceBatch implementation with an injectable quiet-window timer.
// Production passes newRealBatchTimer; tests inject a controllable timer to drive the quiet
// deadline deterministically instead of relying on wall-clock sleeps.
func debounceBatch[T any](p *Pipeline[T], quiet time.Duration, maxItems int, newTimer func() batchTimer) *Pipeline[[]T] {
	if maxItems < 1 {
		maxItems = 1
	}
	return &Pipeline[[]T]{
		create: func(ctx context.Context) Iterator[[]T] {
			source := p.create(ctx)
			debCtx, cancel := context.WithCancel(ctx) //nolint:gosec // cancel is called in debounceBatchIter.Close

			ch := make(chan result[T], 1)
			go func() {
				defer close(ch)
				for {
					val, ok, err := source.Next(debCtx)
					if err != nil {
						select {
						case ch <- result[T]{err: err}:
						case <-debCtx.Done():
						}
						return
					}
					if !ok {
						return
					}
					select {
					case ch <- result[T]{val: val, ok: true}:
					case <-debCtx.Done():
						return
					}
				}
			}()

			return &debounceBatchIter[T]{
				ch:       ch,
				quiet:    quiet,
				maxItems: maxItems,
				timer:    newTimer(),
				cancel:   cancel,
				closer:   source.Close,
			}
		},
	}
}

// batchTimer is the trailing-edge quiet-window timer seam for DebounceBatch. The production
// implementation wraps a *time.Timer; tests inject a controllable timer so the quiet deadline
// fires on demand rather than after a real sleep.
type batchTimer interface {
	// C returns the channel that receives a value when the quiet window elapses.
	C() <-chan time.Time
	// reset arms (or re-arms) the timer for the quiet window, discarding any pending fire.
	reset(quiet time.Duration)
	// stop disarms the timer, discarding any pending fire.
	stop()
}

// realBatchTimer is the production batchTimer backed by a *time.Timer. It is created stopped
// and drained so a quiet stream never observes a spurious fire before the first reset.
type realBatchTimer struct {
	t *time.Timer
}

func newRealBatchTimer() batchTimer {
	t := time.NewTimer(0)
	if !t.Stop() {
		<-t.C
	}
	return &realBatchTimer{t: t}
}

func (r *realBatchTimer) C() <-chan time.Time { return r.t.C }

func (r *realBatchTimer) reset(quiet time.Duration) {
	r.stop()
	r.t.Reset(quiet)
}

func (r *realBatchTimer) stop() {
	if !r.t.Stop() {
		select {
		case <-r.t.C:
		default:
		}
	}
}

type debounceBatchIter[T any] struct {
	ch       <-chan result[T]
	quiet    time.Duration
	maxItems int
	timer    batchTimer
	cancel   context.CancelFunc
	closer   func() error
}

func (it *debounceBatchIter[T]) Next(ctx context.Context) (batch []T, ok bool, err error) {
	// The timer is disarmed while the buffer is empty and only armed once a value arrives,
	// so a quiet stream parks here without ever firing a spurious empty flush.
	it.timer.stop()
	defer it.timer.stop()

	for {
		select {
		case r, open := <-it.ch:
			if !open {
				if len(batch) > 0 {
					return batch, true, nil
				}
				if cerr := ctx.Err(); cerr != nil {
					return nil, false, cerr
				}
				return nil, false, nil
			}
			if r.err != nil {
				return nil, false, r.err
			}
			batch = append(batch, r.val)
			if len(batch) >= it.maxItems {
				return batch, true, nil
			}
			// Reset the trailing-edge window: every value pushes the deadline out.
			it.timer.reset(it.quiet)

		case <-it.timer.C():
			if len(batch) > 0 {
				return batch, true, nil
			}

		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
}

func (it *debounceBatchIter[T]) Close() error {
	it.cancel()
	return it.closer()
}
