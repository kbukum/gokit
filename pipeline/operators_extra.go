package pipeline

import (
	"context"
	"fmt"
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

// Partition splits a pipeline into two streaming branches using predicate.
// The upstream is consumed once by a bounded tee. Both branches should be
// consumed concurrently; closing one branch drops values routed to it while the
// other branch continues.
func Partition[T any](p *Pipeline[T], predicate func(T) bool) (matching *Pipeline[T], rejected *Pipeline[T]) {
	shared := newPartitionState(predicate)
	left := &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			return shared.branch(ctx, partitionMatch, p)
		},
	}
	right := &Pipeline[T]{
		create: func(ctx context.Context) Iterator[T] {
			return shared.branch(ctx, partitionReject, p)
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

type partitionBranch uint8

const (
	partitionMatch partitionBranch = iota
	partitionReject
)

type partitionState[T any] struct {
	predicate func(T) bool

	once       sync.Once
	finishOnce sync.Once

	mu        sync.Mutex
	cancel    context.CancelFunc
	closed    [2]bool
	closeCh   [2]chan struct{}
	out       [2]chan result[T]
	source    Iterator[T]
	sourceSet bool
}

type partitionIter[T any] struct {
	state  *partitionState[T]
	branch partitionBranch
	ctx    context.Context
	p      *Pipeline[T]
	closed bool
}

func newPartitionState[T any](predicate func(T) bool) *partitionState[T] {
	return &partitionState[T]{
		predicate: predicate,
		closeCh:   [2]chan struct{}{make(chan struct{}), make(chan struct{})},
		out:       [2]chan result[T]{make(chan result[T], 1), make(chan result[T], 1)},
	}
}

func (s *partitionState[T]) branch(ctx context.Context, branch partitionBranch, p *Pipeline[T]) Iterator[T] {
	return &partitionIter[T]{state: s, branch: branch, ctx: ctx, p: p}
}

func (it *partitionIter[T]) Next(ctx context.Context) (result T, ok bool, err error) {
	if it.closed {
		var zero T
		return zero, false, nil
	}
	it.state.start(it.ctx, ctx, it.p)
	select {
	case r, open := <-it.state.out[it.branch]:
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

func (it *partitionIter[T]) Close() error {
	if it.closed {
		return nil
	}
	it.closed = true
	return it.state.closeBranch(it.branch)
}

func (s *partitionState[T]) start(createCtx, firstNextCtx context.Context, p *Pipeline[T]) {
	s.once.Do(func() {
		if createCtx == nil {
			createCtx = context.Background() //nolint:contextcheck // nil context guard for custom factories
		}
		teeCtx, cancel := context.WithCancel(createCtx)
		s.mu.Lock()
		s.cancel = cancel
		s.source = p.create(teeCtx)
		s.sourceSet = true
		s.mu.Unlock()

		if firstNextCtx != nil {
			go func() {
				select {
				case <-firstNextCtx.Done():
					cancel()
				case <-teeCtx.Done():
				}
			}()
		}

		go s.consume(teeCtx)
	})
}

func (s *partitionState[T]) consume(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.finish(ctx, result[T]{err: fmt.Errorf("pipeline: partition predicate panic: %v", r)})
		} else {
			s.finish(ctx, result[T]{})
		}
	}()
	defer func() {
		s.mu.Lock()
		source := s.source
		s.mu.Unlock()
		if source != nil {
			_ = source.Close()
		}
	}()

	for {
		val, ok, err := s.source.Next(ctx)
		if err != nil {
			s.finish(ctx, result[T]{err: err})
			return
		}
		if !ok {
			return
		}
		branch := partitionReject
		if s.predicate(val) {
			branch = partitionMatch
		}
		if !s.send(ctx, branch, result[T]{val: val, ok: true}) {
			return
		}
	}
}

func (s *partitionState[T]) send(ctx context.Context, branch partitionBranch, r result[T]) bool {
	idx := int(branch)
	for {
		s.mu.Lock()
		closed := s.closed[idx]
		closeCh := s.closeCh[idx]
		out := s.out[idx]
		s.mu.Unlock()
		if closed {
			return true
		}
		select {
		case out <- r:
			return true
		case <-closeCh:
			return true
		case <-ctx.Done():
			return false
		}
	}
}

func (s *partitionState[T]) finish(ctx context.Context, terminal result[T]) {
	s.finishOnce.Do(func() {
		if terminal.err != nil {
			_ = s.send(ctx, partitionMatch, terminal)
			_ = s.send(ctx, partitionReject, terminal)
		}
		s.mu.Lock()
		cancel := s.cancel
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		close(s.out[partitionMatch])
		close(s.out[partitionReject])
	})
}

func (s *partitionState[T]) closeBranch(branch partitionBranch) error {
	idx := int(branch)
	var source Iterator[T]
	var shouldCloseSource bool
	s.mu.Lock()
	if !s.closed[idx] {
		s.closed[idx] = true
		close(s.closeCh[idx])
	}
	bothClosed := s.closed[partitionMatch] && s.closed[partitionReject]
	if bothClosed && s.cancel != nil {
		s.cancel()
	}
	if bothClosed && s.sourceSet {
		source = s.source
		shouldCloseSource = true
	}
	s.mu.Unlock()
	if shouldCloseSource {
		return source.Close()
	}
	return nil
}
