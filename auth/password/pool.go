package password

import (
	"context"
	"errors"
	"sync"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/util"
)

// ReasonBusy marks a RATE_LIMITED refusal from a saturated [Pool].
const (
	ReasonBusy   = "PASSWORD_BUSY"
	ReasonClosed = "PASSWORD_POOL_CLOSED"
)

// Pool bounds.
const (
	MaxPoolWorkers = 256
	MaxPoolQueue   = 4096
	MaxPoolWait    = 30 * time.Second
)

// PoolConfig bounds concurrent hashing work. Argon2id holds its configured memory per worker, so Workers sets the
// memory ceiling.
type PoolConfig struct {
	// Workers is how many hashes run at once (1-256).
	Workers int
	// Queue is how many callers may wait for a worker (0-4096); zero refuses when every worker is busy.
	Queue int
	// MaxWait is how long a queued caller waits; required when Queue is positive, at most 30 seconds.
	MaxWait time.Duration
}

// Validate checks the bounds.
func (c PoolConfig) Validate() error {
	switch {
	case c.Workers < 1 || c.Workers > MaxPoolWorkers:
		return apperrors.InvalidInput("workers", "workers must be from 1 to 256")
	case c.Queue < 0 || c.Queue > MaxPoolQueue:
		return apperrors.InvalidInput("queue", "queue must be from 0 to 4096")
	case c.Queue > 0 && (c.MaxWait <= 0 || c.MaxWait > MaxPoolWait):
		return apperrors.InvalidInput("max_wait", "max_wait must be from 1ns to 30s when queue is positive")
	case c.Queue == 0 && c.MaxWait != 0:
		return apperrors.InvalidInput("max_wait", "max_wait requires a queue")
	}
	return nil
}

// Pool admits hashing work through a bounded worker pool and queue. A caller beyond the queue, or one that waits
// past MaxWait, gets RATE_LIMITED with ReasonBusy before any hashing starts, so a login flood cannot exhaust memory
// or CPU.
type Pool struct {
	hasher  Hasher
	gate    *resilience.Bulkhead
	mu      sync.Mutex
	closed  bool
	calls   int
	pending map[*context.CancelFunc]struct{}
	done    chan struct{}
}

// NewPool wraps hasher in a bounded pool.
func NewPool(hasher Hasher, cfg PoolConfig) (*Pool, error) {
	if util.IsNil(hasher) {
		return nil, apperrors.InvalidInput("hasher", "Hasher is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	gate, err := resilience.NewBulkhead(resilience.BulkheadConfig{Name: "password", MaxConcurrent: cfg.Workers, MaxWait: cfg.MaxWait, MaxQueue: cfg.Queue})
	if err != nil {
		return nil, err
	}
	return &Pool{hasher: hasher, gate: gate, pending: make(map[*context.CancelFunc]struct{}), done: make(chan struct{})}, nil
}

// Hash runs [Hasher.Hash] once a worker is free.
func (p *Pool) Hash(ctx context.Context, password string) (string, error) {
	var hash string
	err := p.run(ctx, func() (err error) {
		hash, err = p.hasher.Hash(password)
		return err
	})
	if err != nil {
		return "", err
	}
	return hash, nil
}

// Verify runs [Hasher.Verify] once a worker is free.
func (p *Pool) Verify(ctx context.Context, password, hash string) error {
	return p.run(ctx, func() error { return p.hasher.Verify(password, hash) })
}

func (p *Pool) run(ctx context.Context, work func() error) error {
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return poolClosed()
	}
	p.calls++
	p.pending[&cancel] = struct{}{}
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.calls--
		delete(p.pending, &cancel)
		if p.closed && p.calls == 0 {
			close(p.done)
		}
	}()
	var ran bool
	err := p.gate.Execute(workCtx, func() error {
		p.mu.Lock()
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return poolClosed()
		}
		if workCtx.Err() != nil {
			return errCallerDone
		}
		ran = true
		err := work()
		if ctx.Err() != nil {
			// The caller left during non-preemptible work: report why, never the credential outcome.
			return errCallerDone
		}
		return err
	})
	if !ran {
		p.mu.Lock()
		closed := p.closed
		p.mu.Unlock()
		if closed {
			return poolClosed()
		}
	}
	if !ran && (errors.Is(err, resilience.ErrBulkheadFull) || errors.Is(err, resilience.ErrBulkheadTimeout)) {
		return apperrors.RateLimited().WithReason(ReasonBusy).WithCause(err)
	}
	if failure := apperrors.FromContext(ctx, "password"); failure != nil && (errors.Is(err, errCallerDone) || !ran) {
		return failure
	}
	return err
}

// errCallerDone marks work abandoned because the caller's context ended; run replaces it with the classified context failure.
var errCallerDone = errors.New("password caller done")

func poolClosed() error {
	return apperrors.New(apperrors.ErrCodeServiceUnavailable, "Password pool is closed").WithReason(ReasonClosed)
}

// Close refuses new work, cancels queued admission and waits for non-preemptible work. A timed-out Close retains ownership; another Close can finish draining.
func (p *Pool) Close(ctx context.Context) error {
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		for cancel := range p.pending {
			(*cancel)()
		}
		if p.calls == 0 {
			close(p.done)
		}
	}
	p.mu.Unlock()
	select {
	case <-p.done:
		return nil
	default:
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return apperrors.FromContext(ctx, "password.close")
	}
}
