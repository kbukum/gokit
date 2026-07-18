package stream

import (
	"context"
	"fmt"
	"sync"
)

// Partition splits a pipeline into two streaming branches using predicate. The upstream is consumed once by a bounded tee. Both branches should be consumed concurrently; closing one branch drops values routed to it while the other branch continues.
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

type partitionBranch uint8

const (
	partitionMatch partitionBranch = iota
	partitionReject
)

type partitionState[T any] struct {
	predicate func(T) bool

	once            sync.Once
	finishOnce      sync.Once
	sourceCloseOnce sync.Once

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
			s.finish(result[T]{err: fmt.Errorf("stream: partition predicate panic: %v", r)})
		} else {
			s.finish(result[T]{})
		}
	}()
	defer func() { _ = s.closeSource() }()

	for {
		val, ok, err := s.source.Next(ctx)
		if err != nil {
			s.finish(result[T]{err: err})
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

// finish delivers the terminal result to both branches exactly once and closes them. The terminal error is delivered with sendTerminal, which blocks until each branch drains it or is closed. It deliberately does not select on the tee context: when the terminal error is the cancellation itself, that context is already canceled, so a ctx arm would race the delivery and drop the error.
func (s *partitionState[T]) finish(terminal result[T]) {
	s.finishOnce.Do(func() {
		if terminal.err != nil {
			s.sendTerminal(partitionMatch, terminal)
			s.sendTerminal(partitionReject, terminal)
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

// sendTerminal delivers the terminal result to a branch. It blocks until the
// consumer drains the 1-slot buffer (so an already-buffered value is never
// dropped) or the branch is closed. It does not drop buffered data and does not
// select on the tee context (see finish). This cannot deadlock in supported
// usage: every standard consumer (Collect/Drain/ForEach) closes its branch on
// return — including on its own cancellation — which closes closeCh; the Iterator
// contract requires callers to Close(), so a consumer that stops reading without
// closing is a contract violation.
func (s *partitionState[T]) sendTerminal(branch partitionBranch, r result[T]) {
	idx := int(branch)
	s.mu.Lock()
	closed := s.closed[idx]
	closeCh := s.closeCh[idx]
	out := s.out[idx]
	s.mu.Unlock()
	if closed {
		return
	}
	select {
	case out <- r:
	case <-closeCh:
	}
}

func (s *partitionState[T]) closeBranch(branch partitionBranch) error {
	idx := int(branch)
	shouldCloseSource := false
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
		shouldCloseSource = true
	}
	s.mu.Unlock()
	if shouldCloseSource {
		return s.closeSource()
	}
	return nil
}

func (s *partitionState[T]) closeSource() error {
	var err error
	s.sourceCloseOnce.Do(func() {
		s.mu.Lock()
		source := s.source
		s.mu.Unlock()
		if source != nil {
			err = source.Close()
		}
	})
	return err
}
