package provider

import (
	"context"
	"sync"
)

// FanOutStream sends the same input to multiple Stream providers in parallel
// and returns a merged iterator that yields results from all of them.
// Results are emitted in the order they arrive (non-deterministic).
func FanOutStream[I, O any](name string, streams ...Stream[I, O]) Stream[I, O] {
	return &fanOutStream[I, O]{name: name, streams: streams}
}

type fanOutStream[I, O any] struct {
	name    string
	streams []Stream[I, O]
}

func (f *fanOutStream[I, O]) Name() string { return f.name }
func (f *fanOutStream[I, O]) IsAvailable(ctx context.Context) bool {
	for _, s := range f.streams {
		if !s.IsAvailable(ctx) {
			return false
		}
	}
	return len(f.streams) > 0
}

func (f *fanOutStream[I, O]) Execute(ctx context.Context, input I) (Iterator[O], error) {
	iterators := make([]Iterator[O], 0, len(f.streams))
	for _, s := range f.streams {
		iter, err := s.Execute(ctx, input)
		if err != nil {
			// Close any already-opened iterators.
			for _, it := range iterators {
				_ = it.Close()
			}
			return nil, err
		}
		iterators = append(iterators, iter)
	}
	return newMergedIterator[O](ctx, iterators), nil
}

// mergedIterator yields items from multiple iterators via a shared channel.
type mergedIterator[T any] struct {
	ch     chan mergedItem[T]
	cancel context.CancelFunc
	iters  []Iterator[T]
	once   sync.Once
}

type mergedItem[T any] struct {
	val T
	err error
}

func newMergedIterator[T any](parent context.Context, iters []Iterator[T]) *mergedIterator[T] {
	ctx, cancel := context.WithCancel(parent) //nolint:gosec // cancel is called in mergedIterator.Close
	ch := make(chan mergedItem[T], len(iters))

	var wg sync.WaitGroup
	for _, iter := range iters {
		wg.Add(1)
		go func(it Iterator[T]) {
			defer wg.Done()
			for {
				val, ok, err := it.Next(ctx)
				if err != nil {
					ch <- mergedItem[T]{err: err}
					return
				}
				if !ok {
					return
				}
				select {
				case ch <- mergedItem[T]{val: val}:
				case <-ctx.Done():
					return
				}
			}
		}(iter)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	return &mergedIterator[T]{ch: ch, cancel: cancel, iters: iters}
}

func (m *mergedIterator[T]) Next(ctx context.Context) (val T, ok bool, err error) {
	var zero T
	select {
	case item, ok := <-m.ch:
		if !ok {
			return zero, false, nil
		}
		if item.err != nil {
			return zero, false, item.err
		}
		return item.val, true, nil
	case <-ctx.Done():
		return zero, false, ctx.Err()
	}
}

func (m *mergedIterator[T]) Close() error {
	m.once.Do(func() {
		m.cancel()
	})
	var firstErr error
	for _, it := range m.iters {
		if err := it.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// WindowedStream buffers items from a stream into fixed-size windows and
// processes each window as a batch through a RequestResponse provider.
func WindowedStream[I, O, R any](
	name string,
	inner Stream[I, O],
	windowSize int,
	processor RequestResponse[[]O, R],
) Stream[I, R] {
	return &windowedStream[I, O, R]{
		name:       name,
		inner:      inner,
		windowSize: windowSize,
		processor:  processor,
	}
}

type windowedStream[I, O, R any] struct {
	name       string
	inner      Stream[I, O]
	windowSize int
	processor  RequestResponse[[]O, R]
}

func (w *windowedStream[I, O, R]) Name() string { return w.name }
func (w *windowedStream[I, O, R]) IsAvailable(ctx context.Context) bool {
	return w.inner.IsAvailable(ctx) && w.processor.IsAvailable(ctx)
}

func (w *windowedStream[I, O, R]) Execute(ctx context.Context, input I) (Iterator[R], error) {
	iter, err := w.inner.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	return &windowedIterator[O, R]{
		ctx:        ctx,
		inner:      iter,
		windowSize: w.windowSize,
		processor:  w.processor,
	}, nil
}

type windowedIterator[O, R any] struct {
	ctx        context.Context
	inner      Iterator[O]
	windowSize int
	processor  RequestResponse[[]O, R]
	done       bool
}

func (w *windowedIterator[O, R]) Next(ctx context.Context) (val R, ok bool, err error) {
	var zero R
	if w.done {
		return zero, false, nil
	}

	window := make([]O, 0, w.windowSize)
	for len(window) < w.windowSize {
		val, ok, err := w.inner.Next(ctx)
		if err != nil {
			return zero, false, err
		}
		if !ok {
			w.done = true
			break
		}
		window = append(window, val)
	}

	if len(window) == 0 {
		return zero, false, nil
	}

	result, err := w.processor.Execute(ctx, window)
	if err != nil {
		return zero, false, err
	}
	return result, true, nil
}

func (w *windowedIterator[O, R]) Close() error {
	return w.inner.Close()
}

// DrainIterator wraps an iterator so that when Close is called, it drains
// up to maxDrain remaining items before closing the underlying iterator.
// This supports graceful shutdown — processing in-flight items rather than
// dropping them.
func DrainIterator[T any](iter Iterator[T], maxDrain int) Iterator[T] {
	return &drainIterator[T]{inner: iter, maxDrain: maxDrain}
}

type drainIterator[T any] struct {
	inner    Iterator[T]
	maxDrain int
	drained  []T
}

func (d *drainIterator[T]) Next(ctx context.Context) (val T, ok bool, err error) {
	return d.inner.Next(ctx)
}

func (d *drainIterator[T]) Close() error {
	// Drain remaining items before closing.
	ctx := context.Background()
	for i := 0; i < d.maxDrain; i++ {
		val, ok, err := d.inner.Next(ctx)
		if err != nil || !ok {
			break
		}
		d.drained = append(d.drained, val)
	}
	return d.inner.Close()
}

// Drained returns items that were drained during Close.
// Only valid after Close has been called.
func (d *drainIterator[T]) Drained() []T {
	return d.drained
}
