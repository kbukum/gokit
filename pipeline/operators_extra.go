package pipeline

import (
	"context"
	"sync"
)

// Distinct removes duplicate comparable values while preserving first-seen order.
func Distinct[T comparable](p *Pipeline[T]) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			return &distinctIter[T]{source: p.create(ctx), seen: make(map[T]struct{})}
		},
	}
}

// Take yields at most the first n values from the pipeline.
func Take[T any](p *Pipeline[T], n int) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			return &takeIter[T]{source: p.create(ctx), remaining: n}
		},
	}
}

// Skip ignores the first n values from the pipeline.
func Skip[T any](p *Pipeline[T], n int) *Pipeline[T] {
	return &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			return &skipIter[T]{source: p.create(ctx), remaining: n}
		},
	}
}

// Partition splits a pipeline into two pipelines using predicate.
func Partition[T any](p *Pipeline[T], predicate func(T) bool) (matching *Pipeline[T], rejected *Pipeline[T]) {
	shared := &partitionState[T]{predicate: predicate}
	left := &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			shared.partition(ctx, p)
			return &partitionSliceIter[T]{items: shared.matching, err: shared.err}
		},
	}
	right := &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			shared.partition(ctx, p)
			return &partitionSliceIter[T]{items: shared.rejected, err: shared.err}
		},
	}
	return left, right
}

type distinctIter[T comparable] struct {
	source Iterator[T]
	seen   map[T]struct{}
}

func (it *distinctIter[T]) Next(ctx context.Context) (result T, ok bool, err error) {
	for {
		val, ok, err := it.source.Next(ctx)
		if err != nil || !ok {
			return val, ok, err
		}
		if _, exists := it.seen[val]; exists {
			continue
		}
		it.seen[val] = struct{}{}
		return val, true, nil
	}
}

func (it *distinctIter[T]) Close() error { return it.source.Close() }

type takeIter[T any] struct {
	source    Iterator[T]
	remaining int
}

func (it *takeIter[T]) Next(ctx context.Context) (result T, ok bool, err error) {
	if it.remaining <= 0 {
		var zero T
		return zero, false, nil
	}
	val, ok, err := it.source.Next(ctx)
	if err != nil || !ok {
		return val, ok, err
	}
	it.remaining--
	return val, true, nil
}

func (it *takeIter[T]) Close() error { return it.source.Close() }

type skipIter[T any] struct {
	source    Iterator[T]
	remaining int
}

func (it *skipIter[T]) Next(ctx context.Context) (result T, ok bool, err error) {
	for it.remaining > 0 {
		_, ok, err := it.source.Next(ctx)
		if err != nil || !ok {
			var zero T
			return zero, false, err
		}
		it.remaining--
	}
	return it.source.Next(ctx)
}

func (it *skipIter[T]) Close() error { return it.source.Close() }

type partitionState[T any] struct {
	once      sync.Once
	predicate func(T) bool
	matching  []T
	rejected  []T
	err       error
}

type partitionSliceIter[T any] struct {
	items   []T
	index   int
	err     error
	errSent bool
}

func (it *partitionSliceIter[T]) Next(_ context.Context) (result T, ok bool, err error) {
	if it.index < len(it.items) {
		val := it.items[it.index]
		it.index++
		return val, true, nil
	}
	if it.err != nil && !it.errSent {
		it.errSent = true
		var zero T
		return zero, false, it.err
	}
	var zero T
	return zero, false, nil
}

func (it *partitionSliceIter[T]) Close() error { return nil }

func (s *partitionState[T]) partition(ctx context.Context, p *Pipeline[T]) {
	s.once.Do(func() {
		iter := p.create(ctx)
		defer func() { _ = iter.Close() }()
		for {
			val, ok, err := iter.Next(ctx)
			if err != nil {
				s.err = err
				return
			}
			if !ok {
				return
			}
			if s.predicate(val) {
				s.matching = append(s.matching, val)
				continue
			}
			s.rejected = append(s.rejected, val)
		}
	})
}
