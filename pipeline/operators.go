package pipeline

import (
	"context"
	"sync"
)

// Map transforms each value using fn.
func Map[I, O any](p *Pipeline[I], fn func(context.Context, I) (O, error)) *Pipeline[O] {
	return &Pipeline[O]{
		create: func(ctx context.Context) Iterator[O] {
			return &mapIter[I, O]{source: p.create(ctx), fn: fn}
		},
	}
}

// FlatMap transforms each value into an iterator and flattens the results.
func FlatMap[I, O any](p *Pipeline[I], fn func(context.Context, I) (Iterator[O], error)) *Pipeline[O] {
	return &Pipeline[O]{
		create: func(ctx context.Context) Iterator[O] {
			return &flatMapIter[I, O]{source: p.create(ctx), fn: fn}
		},
	}
}

// Filter keeps only values that satisfy the predicate.
func Filter[T any](p *Pipeline[T], fn func(T) bool) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			return &filterIter[T]{source: p.create(ctx), fn: fn}
		},
	}
}

// Tap calls fn as a side-effect for each value, then passes the value through unchanged.
// Use for logging, metrics, or mid-pipeline publishing.
func Tap[T any](p *Pipeline[T], fn func(context.Context, T) error) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			return &tapIter[T]{source: p.create(ctx), fn: fn}
		},
	}
}

// TapEach applies fn[i] to each element of a []T slice as a side-effect,
// then passes the slice through unchanged. Useful after FanOut.
func TapEach[T any](p *Pipeline[[]T], fns ...func(context.Context, T) error) *Pipeline[[]T] {
	return &Pipeline[[]T]{
		create: func(ctx context.Context) Iterator[[]T] {
			return &tapEachIter[T]{source: p.create(ctx), fns: fns}
		},
	}
}

// FanOut applies multiple functions to each input value in parallel
// and collects all results as a slice.
func FanOut[I, O any](p *Pipeline[I], fns ...func(context.Context, I) (O, error)) *Pipeline[[]O] {
	return &Pipeline[[]O]{
		create: func(ctx context.Context) Iterator[[]O] {
			return &fanOutIter[I, O]{source: p.create(ctx), fns: fns}
		},
	}
}

// Reduce accumulates all values into a single result.
// The pipeline yields exactly one value: the final accumulator.
func Reduce[T, R any](p *Pipeline[T], init R, fn func(R, T) R) *Pipeline[R] {
	return &Pipeline[R]{
		create: func(ctx context.Context) Iterator[R] {
			return &reduceIter[T, R]{source: p.create(ctx), acc: init, fn: fn}
		},
	}
}

// Concat joins multiple pipelines sequentially.
// All values from the first pipeline are yielded before the second, etc.
func Concat[T any](pipelines ...*Pipeline[T]) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			iters := make([]Iterator[T], len(pipelines))
			for i, p := range pipelines {
				iters[i] = p.create(ctx)
			}
			return &concatIter[T]{iters: iters}
		},
	}
}

// --- Iterator implementations ---

type mapIter[I, O any] struct {
	source Iterator[I]
	fn     func(context.Context, I) (O, error)
}

func (it *mapIter[I, O]) Next(ctx context.Context) (result O, ok bool, err error) {
	val, ok, err := it.source.Next(ctx)
	if err != nil || !ok {
		var zero O
		return zero, false, err
	}
	out, err := it.fn(ctx, val)
	if err != nil {
		var zero O
		return zero, false, err
	}
	return out, true, nil
}

func (it *mapIter[I, O]) Close() error { return it.source.Close() }

type flatMapIter[I, O any] struct {
	source  Iterator[I]
	fn      func(context.Context, I) (Iterator[O], error)
	current Iterator[O]
}

func (it *flatMapIter[I, O]) Next(ctx context.Context) (result O, ok bool, err error) {
	for {
		if it.current != nil {
			val, ok, err := it.current.Next(ctx)
			if err != nil {
				var zero O
				return zero, false, err
			}
			if ok {
				return val, true, nil
			}
			_ = it.current.Close()
			it.current = nil
		}
		in, ok, err := it.source.Next(ctx)
		if err != nil || !ok {
			var zero O
			return zero, false, err
		}
		inner, err := it.fn(ctx, in)
		if err != nil {
			var zero O
			return zero, false, err
		}
		it.current = inner
	}
}

func (it *flatMapIter[I, O]) Close() error {
	if it.current != nil {
		_ = it.current.Close()
	}
	return it.source.Close()
}

type filterIter[T any] struct {
	source Iterator[T]
	fn     func(T) bool
}

func (it *filterIter[T]) Next(ctx context.Context) (result T, ok bool, err error) {
	for {
		val, ok, err := it.source.Next(ctx)
		if err != nil || !ok {
			return val, false, err
		}
		if it.fn(val) {
			return val, true, nil
		}
	}
}

func (it *filterIter[T]) Close() error { return it.source.Close() }

type tapIter[T any] struct {
	source Iterator[T]
	fn     func(context.Context, T) error
}

func (it *tapIter[T]) Next(ctx context.Context) (result T, ok bool, err error) {
	val, ok, err := it.source.Next(ctx)
	if err != nil || !ok {
		return val, ok, err
	}
	if err := it.fn(ctx, val); err != nil {
		var zero T
		return zero, false, err
	}
	return val, true, nil
}

func (it *tapIter[T]) Close() error { return it.source.Close() }

type tapEachIter[T any] struct {
	source Iterator[[]T]
	fns    []func(context.Context, T) error
}

func (it *tapEachIter[T]) Next(ctx context.Context) (result []T, ok bool, err error) {
	vals, ok, err := it.source.Next(ctx)
	if err != nil || !ok {
		return vals, ok, err
	}
	n := len(it.fns)
	if n > len(vals) {
		n = len(vals)
	}
	for i := 0; i < n; i++ {
		if err := it.fns[i](ctx, vals[i]); err != nil {
			return nil, false, err
		}
	}
	return vals, true, nil
}

func (it *tapEachIter[T]) Close() error { return it.source.Close() }

type fanOutIter[I, O any] struct {
	source Iterator[I]
	fns    []func(context.Context, I) (O, error)
}

func (it *fanOutIter[I, O]) Next(ctx context.Context) (result []O, ok bool, err error) {
	val, ok, err := it.source.Next(ctx)
	if err != nil || !ok {
		return nil, false, err
	}
	results := make([]O, len(it.fns))
	errs := make([]error, len(it.fns))
	var wg sync.WaitGroup
	wg.Add(len(it.fns))
	for i, fn := range it.fns {
		go func(i int, fn func(context.Context, I) (O, error)) {
			defer wg.Done()
			results[i], errs[i] = fn(ctx, val)
		}(i, fn)
	}
	wg.Wait()
	for _, e := range errs {
		if e != nil {
			return nil, false, e
		}
	}
	return results, true, nil
}

func (it *fanOutIter[I, O]) Close() error { return it.source.Close() }

type reduceIter[T, R any] struct {
	source Iterator[T]
	acc    R
	fn     func(R, T) R
	done   bool
}

func (it *reduceIter[T, R]) Next(ctx context.Context) (result R, ok bool, err error) {
	if it.done {
		var zero R
		return zero, false, nil
	}
	for {
		val, ok, err := it.source.Next(ctx)
		if err != nil {
			var zero R
			return zero, false, err
		}
		if !ok {
			it.done = true
			return it.acc, true, nil
		}
		it.acc = it.fn(it.acc, val)
	}
}

func (it *reduceIter[T, R]) Close() error { return it.source.Close() }

type concatIter[T any] struct {
	iters []Iterator[T]
	index int
}

func (it *concatIter[T]) Next(ctx context.Context) (result T, ok bool, err error) {
	for it.index < len(it.iters) {
		val, ok, err := it.iters[it.index].Next(ctx)
		if err != nil {
			return val, false, err
		}
		if ok {
			return val, true, nil
		}
		it.index++
	}
	var zero T
	return zero, false, nil
}

func (it *concatIter[T]) Close() error {
	var firstErr error
	for _, iter := range it.iters {
		if err := iter.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
