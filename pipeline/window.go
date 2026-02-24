package pipeline

import (
	"context"
	"time"
)

// TumblingWindow groups values into non-overlapping fixed-duration windows.
// Each window is emitted as a slice when its duration expires.
// The final partial window is emitted when the source is exhausted.
func TumblingWindow[T any](p *Pipeline[T], duration time.Duration) *Pipeline[[]T] {
	return &Pipeline[[]T]{
		create: func(ctx context.Context) Iterator[[]T] {
			source := p.create(ctx)
			winCtx, cancel := context.WithCancel(ctx)

			ch := make(chan result[T], 1)
			go func() {
				defer close(ch)
				for {
					val, ok, err := source.Next(winCtx)
					if err != nil {
						select {
						case ch <- result[T]{err: err}:
						case <-winCtx.Done():
						}
						return
					}
					if !ok {
						return
					}
					select {
					case ch <- result[T]{val: val, ok: true}:
					case <-winCtx.Done():
						return
					}
				}
			}()

			return &tumblingWindowIter[T]{
				ch:       ch,
				duration: duration,
				cancel:   cancel,
				closer:   source.Close,
			}
		},
	}
}

type tumblingWindowIter[T any] struct {
	ch       <-chan result[T]
	duration time.Duration
	cancel   context.CancelFunc
	closer   func() error
	done     bool
}

func (it *tumblingWindowIter[T]) Next(ctx context.Context) (result []T, ok bool, err error) {
	if it.done {
		return nil, false, nil
	}

	var window []T
	timer := time.NewTimer(it.duration)
	defer timer.Stop()

	for {
		select {
		case r, open := <-it.ch:
			if !open {
				it.done = true
				if len(window) > 0 {
					return window, true, nil
				}
				return nil, false, nil
			}
			if r.err != nil {
				return nil, false, r.err
			}
			window = append(window, r.val)

		case <-timer.C:
			if len(window) > 0 {
				return window, true, nil
			}
			// Empty window — reset and keep waiting
			timer.Reset(it.duration)

		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
}

func (it *tumblingWindowIter[T]) Close() error {
	it.cancel()
	return it.closer()
}

// SlidingWindow emits overlapping windows based on a time extraction function.
// windowSize is the duration of each window. slideBy is how far each window advances.
//
// Values must arrive in time order. Each emitted slice contains all values
// whose timestamp falls within [windowStart, windowStart+windowSize).
func SlidingWindow[T any](p *Pipeline[T], timeFn func(T) time.Time, windowSize, slideBy time.Duration) *Pipeline[[]T] {
	return &Pipeline[[]T]{
		create: func(ctx context.Context) Iterator[[]T] {
			return &slidingWindowIter[T]{
				source:     p.create(ctx),
				timeFn:     timeFn,
				windowSize: windowSize,
				slideBy:    slideBy,
			}
		},
	}
}

type slidingWindowIter[T any] struct {
	source     Iterator[T]
	timeFn     func(T) time.Time
	windowSize time.Duration
	slideBy    time.Duration
	buffer     []T
	windowEnd  time.Time
	started    bool
	done       bool
}

func (it *slidingWindowIter[T]) Next(ctx context.Context) (result []T, ok bool, err error) {
	if it.done {
		return nil, false, nil
	}

	for {
		// Fill buffer until we have enough for a window
		for {
			val, ok, err := it.source.Next(ctx)
			if err != nil {
				return nil, false, err
			}
			if !ok {
				it.done = true
				break
			}
			ts := it.timeFn(val)
			if !it.started {
				it.windowEnd = ts.Add(it.windowSize)
				it.started = true
			}
			it.buffer = append(it.buffer, val)

			// If this value is past current window end, emit the window
			if !ts.Before(it.windowEnd) {
				break
			}
		}

		if !it.started || len(it.buffer) == 0 {
			return nil, false, nil
		}

		// Collect values in current window
		windowStart := it.windowEnd.Add(-it.windowSize)
		var window []T
		for _, v := range it.buffer {
			ts := it.timeFn(v)
			if !ts.Before(windowStart) && ts.Before(it.windowEnd) {
				window = append(window, v)
			}
		}

		// Advance window
		it.windowEnd = it.windowEnd.Add(it.slideBy)

		// Remove values that are before the new window start
		newStart := it.windowEnd.Add(-it.windowSize)
		kept := it.buffer[:0]
		for _, v := range it.buffer {
			if !it.timeFn(v).Before(newStart) {
				kept = append(kept, v)
			}
		}
		it.buffer = kept

		if len(window) > 0 {
			return window, true, nil
		}

		if it.done {
			return nil, false, nil
		}
	}
}

func (it *slidingWindowIter[T]) Close() error { return it.source.Close() }
