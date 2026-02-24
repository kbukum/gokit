package pipeline

import (
	"context"
	"time"
)

// Debounce waits for silence of the given duration after the last value
// before emitting. If a new value arrives during the quiet period, the
// timer resets and only the latest value is emitted.
//
// Useful for "wait until input stops" patterns (e.g., search-as-you-type,
// batching rapid events).
func Debounce[T any](p *Pipeline[T], duration time.Duration) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			source := p.create(ctx)
			debCtx, cancel := context.WithCancel(ctx)

			// Feed source values into a channel so we can select on them
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

			return &debounceIter[T]{
				ch:       ch,
				duration: duration,
				cancel:   cancel,
				closer:   source.Close,
			}
		},
	}
}

type debounceIter[T any] struct {
	ch       <-chan result[T]
	duration time.Duration
	cancel   context.CancelFunc
	closer   func() error
}

func (it *debounceIter[T]) Next(ctx context.Context) (result T, ok bool, err error) {
	var latest T
	hasValue := false
	timer := time.NewTimer(it.duration)
	defer timer.Stop()

	for {
		select {
		case r, open := <-it.ch:
			if !open {
				if hasValue {
					return latest, true, nil
				}
				var zero T
				return zero, false, nil
			}
			if r.err != nil {
				return r.val, false, r.err
			}
			latest = r.val
			hasValue = true
			// Reset the timer — new value arrived
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(it.duration)

		case <-timer.C:
			if hasValue {
				return latest, true, nil
			}
			// No value yet, keep waiting
			timer.Reset(it.duration)

		case <-ctx.Done():
			var zero T
			return zero, false, ctx.Err()
		}
	}
}

func (it *debounceIter[T]) Close() error {
	it.cancel()
	return it.closer()
}
