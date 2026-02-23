package pipeline

import "context"

// Iterator provides pull-based sequential access to a stream of values.
// Structurally compatible with provider.Iterator[T].
type Iterator[T any] interface {
	// Next returns the next value. Returns (zero, false, nil) when exhausted.
	Next(ctx context.Context) (T, bool, error)
	// Close releases any resources held by the iterator.
	Close() error
}

// Pipeline represents a lazy, pull-based data pipeline.
// No work happens until values are pulled via Collect, Drain, or ForEach.
type Pipeline[T any] struct {
	create func(ctx context.Context) Iterator[T]
}

// Runnable is a fully-configured pipeline ready to execute.
type Runnable struct {
	run func(ctx context.Context) error
}

// Run executes the pipeline until completion or context cancellation.
func (r *Runnable) Run(ctx context.Context) error {
	return r.run(ctx)
}

// result carries a value or error through a channel.
type result[T any] struct {
	val T
	ok  bool
	err error
}

// channelIter reads values from a channel. Used by concurrent operators.
type channelIter[T any] struct {
	ch     <-chan result[T]
	closer func() error
}

func (it *channelIter[T]) Next(ctx context.Context) (T, bool, error) {
	select {
	case r, open := <-it.ch:
		if !open {
			var zero T
			return zero, false, nil
		}
		return r.val, r.ok, r.err
	case <-ctx.Done():
		var zero T
		return zero, false, ctx.Err()
	}
}

func (it *channelIter[T]) Close() error {
	if it.closer != nil {
		return it.closer()
	}
	return nil
}

// --- Constructors ---

// From creates a pipeline from an existing Iterator.
func From[T any](iter Iterator[T]) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(_ context.Context) Iterator[T] {
			return iter
		},
	}
}

// FromSlice creates a pipeline from a slice of values.
func FromSlice[T any](items []T) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(_ context.Context) Iterator[T] {
			return &sliceIter[T]{items: items}
		},
	}
}

// FromFunc creates a pipeline from a factory that produces an Iterator.
func FromFunc[T any](fn func(ctx context.Context) Iterator[T]) *Pipeline[T] {
	return &Pipeline[T]{create: fn}
}

// --- Terminals ---

// Drain creates a Runnable that pulls all values and sends each to sink.
func Drain[T any](p *Pipeline[T], sink func(context.Context, T) error) *Runnable {
	return &Runnable{
		run: func(ctx context.Context) error {
			iter := p.create(ctx)
			defer iter.Close()
			for {
				val, ok, err := iter.Next(ctx)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
				if err := sink(ctx, val); err != nil {
					return err
				}
			}
		},
	}
}

// Collect runs the pipeline and returns all values as a slice.
func Collect[T any](ctx context.Context, p *Pipeline[T]) ([]T, error) {
	iter := p.create(ctx)
	defer iter.Close()
	var result []T
	for {
		val, ok, err := iter.Next(ctx)
		if err != nil {
			return result, err
		}
		if !ok {
			return result, nil
		}
		result = append(result, val)
	}
}

// ForEach pulls all values and calls fn for each. Convenience wrapper around Drain.
func ForEach[T any](ctx context.Context, p *Pipeline[T], fn func(context.Context, T) error) error {
	return Drain(p, fn).Run(ctx)
}

// Iter returns the raw Iterator for this pipeline. The caller must Close() it.
func (p *Pipeline[T]) Iter(ctx context.Context) Iterator[T] {
	return p.create(ctx)
}

// --- Internal iterators ---

type sliceIter[T any] struct {
	items []T
	index int
}

func (it *sliceIter[T]) Next(_ context.Context) (T, bool, error) {
	if it.index >= len(it.items) {
		var zero T
		return zero, false, nil
	}
	val := it.items[it.index]
	it.index++
	return val, true, nil
}

func (it *sliceIter[T]) Close() error { return nil }
