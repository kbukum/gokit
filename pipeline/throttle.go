package pipeline

import (
	"context"
	"time"
)

// Throttle drops values that arrive faster than the given interval.
// Only the first value in each interval window is emitted; subsequent
// values within the same window are dropped.
// Useful for rate-limiting downstream processing.
func Throttle[T any](p *Pipeline[T], interval time.Duration) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			return &throttleIter[T]{
				source:   p.create(ctx),
				interval: interval,
			}
		},
	}
}

type throttleIter[T any] struct {
	source   Iterator[T]
	interval time.Duration
	lastEmit time.Time
}

func (it *throttleIter[T]) Next(ctx context.Context) (result T, ok bool, err error) {
	for {
		val, ok, err := it.source.Next(ctx)
		if err != nil || !ok {
			return val, ok, err
		}
		now := time.Now()
		if it.lastEmit.IsZero() || now.Sub(it.lastEmit) >= it.interval {
			it.lastEmit = now
			return val, true, nil
		}
		// Drop value — too soon
	}
}

func (it *throttleIter[T]) Close() error { return it.source.Close() }
