package lease

import (
	"context"
	"errors"
	"sync"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Reasons carried by [Set.Acquire] failures.
const (
	ReasonCapacity = "LEASE_CAPACITY"
	ReasonClosed   = "LEASE_CLOSED"
	ReasonExpired  = "LEASE_EXPIRED"
)

// Causes reported by context.Cause on an ended lifetime. An owner cancellation reports the owner's own cause.
var (
	ErrRevoked  = errors.New("lease revoked")
	ErrExpired  = errors.New("lease expired without renewal")
	ErrEnded    = errors.New("lease reached the end of its lifetime")
	ErrReleased = errors.New("lease released")
	ErrClosed   = errors.New("lease set closed")
)

// Grant describes the admitting authority check.
type Grant struct {
	// CheckedAt is read from the set's [Config.Clock] immediately before the admitting check started; with the default
	// monotonic clock it must keep its monotonic reading.
	CheckedAt time.Time

	// Lifetime is the non-renewable bound from CheckedAt, such as the credential's remaining validity.
	Lifetime time.Duration
}

type entry[K comparable] struct {
	key      K
	deadline time.Time
	end      time.Time
	cancel   context.CancelCauseFunc
	stop     func() bool
}

// Set owns leases, their renewal worker and their expiry worker. It is safe for concurrent use; Close it when done.
type Set[K comparable] struct {
	cfg    Config[K]
	timing util.TimerClock

	mu     sync.Mutex
	leases map[*entry[K]]struct{}
	closed bool

	wake  chan struct{}
	nudge chan struct{}
	ctx   context.Context
	stop  context.CancelFunc
	done  chan struct{}
}

// New validates cfg and starts the workers.
func New[K comparable](cfg Config[K]) (*Set[K], error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	ctx, stop := context.WithCancel(context.Background()) //nolint:gosec // G118: stop is owned by the Set and called by Close.
	if cfg.Clock == nil {
		cfg.Clock = util.MonotonicClock{}
	}
	s := &Set[K]{cfg: cfg, timing: cfg.Clock, leases: make(map[*entry[K]]struct{}), wake: make(chan struct{}, 1), nudge: make(chan struct{}, 1), ctx: ctx, stop: stop, done: make(chan struct{})}
	var workers sync.WaitGroup
	workers.Go(s.renewLoop)
	workers.Go(s.expiryLoop)
	go func() { workers.Wait(); close(s.done) }()
	return s, nil
}

// Acquire starts a lease for key after an admitting check described by grant. The returned lifetime ends when the
// lease is denied, expires, reaches its lifetime, is released or revoked, the set closes, or owner ends; context.Cause
// reports which. The owner must call release, which is idempotent.
func (s *Set[K]) Acquire(owner context.Context, key K, grant Grant) (context.Context, func(), error) {
	if err := owner.Err(); err != nil {
		return nil, nil, apperrors.Canceled("lease.acquire").WithCause(context.Cause(owner))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, apperrors.New(apperrors.ErrCodeServiceUnavailable, "Lease set is closed").WithReason(ReasonClosed)
	}
	now := s.timing.Now()
	if grant.CheckedAt.After(now) {
		return nil, nil, apperrors.InvalidInput("lease.checked_at", "admitting check cannot start in the future")
	}
	deadline, end := grant.CheckedAt.Add(s.cfg.Lease), grant.CheckedAt.Add(grant.Lifetime)
	if !now.Before(deadline) || !now.Before(end) {
		return nil, nil, apperrors.New(apperrors.ErrCodeUnauthorized, "Authority expired before the lease started").WithReason(ReasonExpired)
	}
	if len(s.leases) >= s.cfg.Limit {
		return nil, nil, apperrors.New(apperrors.ErrCodeRateLimited, "Lease capacity reached").WithReason(ReasonCapacity)
	}
	life, cancel := context.WithCancelCause(owner)
	e := &entry[K]{key: key, deadline: deadline, end: end, cancel: cancel}
	s.leases[e] = struct{}{}
	// An owner cancellation frees the slot without waiting for expiry.
	e.stop = context.AfterFunc(life, func() { s.end(e, nil) })
	s.signal()
	var once sync.Once
	return life, func() { once.Do(func() { s.end(e, ErrReleased) }) }, nil
}

// end removes e and cancels its lifetime with cause. A nil cause only removes an already ended lease.
func (s *Set[K]) end(e *entry[K], cause error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endLocked(e, cause)
}

func (s *Set[K]) endLocked(e *entry[K], cause error) {
	delete(s.leases, e)
	if cause != nil {
		e.stop()
		e.cancel(cause)
	}
}

// Revoke ends every lease whose key matches before returning and reports how many ended. match runs outside the
// set's lock, so it may be slow or call back into the set; leases acquired while it runs are not considered.
func (s *Set[K]) Revoke(match func(K) bool) int {
	s.mu.Lock()
	snapshot := make([]*entry[K], 0, len(s.leases))
	for e := range s.leases {
		snapshot = append(snapshot, e)
	}
	s.mu.Unlock()
	matched := snapshot[:0]
	for _, e := range snapshot {
		if match(e.key) {
			matched = append(matched, e)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range matched {
		if _, live := s.leases[e]; live {
			s.endLocked(e, ErrRevoked)
			n++
		}
	}
	return n
}

// Len reports the number of live leases.
func (s *Set[K]) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.leases)
}

// Nudge requests an early owned renewal. Requests coalesce; no caller performs or waits for authority work.
func (s *Set[K]) Nudge() {
	select {
	case s.nudge <- struct{}{}:
	default:
	}
}

func (s *Set[K]) renew(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	snapshot := make([]*entry[K], 0, len(s.leases))
	seen := make(map[K]struct{}, len(s.leases))
	keys := make([]K, 0, len(s.leases))
	for e := range s.leases {
		snapshot = append(snapshot, e)
		if _, ok := seen[e.key]; !ok {
			seen[e.key] = struct{}{}
			keys = append(keys, e.key)
		}
	}
	s.mu.Unlock()
	if len(keys) == 0 {
		return nil
	}
	start := s.timing.Now()
	checkCtx, cancel := context.WithTimeout(ctx, s.cfg.Lease)
	defer cancel()
	answers, err := s.cfg.Checker.Check(checkCtx, keys)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.timing.Now()
	renewed := start.Add(s.cfg.Lease)
	for _, e := range snapshot {
		if _, live := s.leases[e]; !live {
			continue
		}
		switch {
		case s.closed:
			continue
		case !now.Before(e.end):
			s.endLocked(e, ErrEnded)
			continue
		case !now.Before(e.deadline) || !now.Before(renewed):
			s.endLocked(e, ErrExpired)
			continue
		case ctx.Err() != nil:
			continue
		}
		ok, answered := answers[e.key]
		switch {
		case !answered:
		case !ok:
			s.endLocked(e, ErrRevoked)
		case renewed.After(e.deadline):
			e.deadline = renewed
		}
	}
	s.signal()
	return err
}

func (s *Set[K]) renewLoop() {
	for {
		start := s.timing.Now()
		if err := s.renew(s.ctx); err != nil && s.ctx.Err() == nil {
			reportCtx, cancel := context.WithTimeout(s.ctx, s.cfg.Lease)
			s.cfg.ReportError(reportCtx, err)
			cancel()
		}
		timer := s.timing.NewTimer(max(start.Add(s.cfg.Interval).Sub(s.timing.Now()), 0))
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C():
		case <-s.nudge:
			timer.Stop()
		}
	}
}

func (s *Set[K]) expiryLoop() {
	for {
		timer := s.timing.NewTimer(s.expire())
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-s.wake:
			timer.Stop()
		case <-timer.C():
		}
	}
}

// expire ends due leases and returns the delay until the next deadline.
func (s *Set[K]) expire() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.timing.Now()
	next := s.cfg.Lease
	for e := range s.leases {
		switch {
		case !now.Before(e.end):
			s.endLocked(e, ErrEnded)
		case !now.Before(e.deadline):
			s.endLocked(e, ErrExpired)
		default:
			next = min(next, e.deadline.Sub(now), e.end.Sub(now))
		}
	}
	return next
}

func (s *Set[K]) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Close ends every lease with [ErrClosed], refuses new ones and waits for the workers until ctx ends. It is idempotent.
func (s *Set[K]) Close(ctx context.Context) error {
	s.mu.Lock()
	s.closed = true
	for e := range s.leases {
		s.endLocked(e, ErrClosed)
	}
	s.mu.Unlock()
	s.stop()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
