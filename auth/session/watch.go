package session

import (
	"context"
	"sync"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
)

type watch struct {
	reference, family string
	expires, lease    time.Time
	absoluteExpiry    time.Time
	ctx               context.Context
	cancel            context.CancelFunc
}

// Acquire performs a final authoritative lookup under the local revocation gate.
// The returned lifetime is canceled by expiry, logout, rotation, store failure or lease exhaustion.
// Release is idempotent and must be called by the stream owner.
func (m *Manager) Acquire(ctx context.Context, ref string) (context.Context, func(), error) {
	admission, cancel := context.WithTimeout(ctx, LookupBudget)
	defer cancel()
	value, err := admitted(admission, m, func() (acquisition, error) { return m.acquire(admission, ctx, ref) })
	return value.lifetime, value.release, err
}

func (m *Manager) acquire(admission, owner context.Context, ref string) (acquisition, error) {
	start := m.timing.Now()
	row, err := m.lookup(admission, ref)
	if err != nil {
		return acquisition{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || admission.Err() != nil || owner.Err() != nil {
		return acquisition{}, auth.Failure("SESSION_CLOSED")
	}
	if len(m.watches) >= WatchLimit {
		return acquisition{}, apperrors.New(apperrors.ErrCodeRateLimited, "Session stream capacity reached").WithReason("SESSION_WATCH_CAPACITY")
	}
	if !m.timing.Now().Before(start.Add(3 * time.Second)) {
		return acquisition{}, auth.Failure("SESSION_LEASE_EXPIRED")
	}
	remaining := row.ExpiresAt.Sub(m.clock.Now())
	if remaining <= 0 {
		return acquisition{}, auth.Failure("SESSION_INVALID")
	}
	life, cancel := context.WithCancel(owner)
	m.sequence++
	id := m.sequence
	m.watches[id] = &watch{reference: ref, family: row.Family, expires: start.Add(min(remaining, Lifetime)), absoluteExpiry: row.ExpiresAt, lease: start.Add(3 * time.Second), ctx: life, cancel: cancel}
	m.signal()
	var once sync.Once
	release := func() { once.Do(func() { m.mu.Lock(); defer m.mu.Unlock(); cancel(); delete(m.watches, id) }) }
	return acquisition{lifetime: life, release: release}, nil
}

func (m *Manager) cancelReference(ref string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, w := range m.watches {
		if w.reference == ref {
			w.cancel()
			delete(m.watches, id)
		}
	}
}

func (m *Manager) cancelFamily(family string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, w := range m.watches {
		if w.family == family {
			w.cancel()
			delete(m.watches, id)
		}
	}
}

// Poll revalidates one bounded watch snapshot. Lease expiry is independently enforced while reads block.
func (m *Manager) Poll(ctx context.Context) {
	m.mu.Lock()
	type entry struct {
		id    uint64
		watch *watch
	}
	refs := make(map[string][]entry, len(m.watches))
	for id, w := range m.watches {
		refs[w.reference] = append(refs[w.reference], entry{id: id, watch: w})
	}
	m.mu.Unlock()
	for ref, entries := range refs {
		if ctx.Err() != nil {
			return
		}
		m.mu.Lock()
		present := false
		for _, entry := range entries {
			if m.watches[entry.id] == entry.watch {
				present = true
				break
			}
		}
		m.mu.Unlock()
		if !present {
			continue
		}
		start := m.timing.Now()
		row, err := m.lookup(ctx, ref)
		m.mu.Lock()
		for _, entry := range entries {
			id, w := entry.id, entry.watch
			if m.watches[id] != w {
				continue
			}
			if err != nil || row.Family != w.family || !m.timing.Now().Before(w.lease) || !m.timing.Now().Before(start.Add(3*time.Second)) {
				w.cancel()
				delete(m.watches, id)
			} else {
				w.lease = start.Add(3 * time.Second)
			}
		}
		m.mu.Unlock()
		m.signal()
	}
}

func (m *Manager) expire() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.timing.Now()
	domainNow := m.clock.Now()
	for id, w := range m.watches {
		if w.ctx.Err() != nil || !now.Before(w.expires) || !now.Before(w.lease) || !domainNow.Before(w.absoluteExpiry) {
			w.cancel()
			delete(m.watches, id)
		}
	}
}

func (m *Manager) signal() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

func (m *Manager) nextExpiry() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.timing.Now()
	delay := time.Second
	for _, w := range m.watches {
		for _, deadline := range []time.Time{w.expires, w.lease} {
			if remaining := deadline.Sub(now); remaining < delay {
				delay = remaining
			}
		}
	}
	return max(delay, 0)
}

// Cleanup performs exactly one indexed batch of at most 256 retained generations.
func (m *Manager) Cleanup(ctx context.Context) (int64, error) {
	if err := m.ready(); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, LookupBudget)
	defer cancel()
	n, err := m.store.Cleanup(ctx, m.clock.Now(), 256)
	if err != nil {
		return 0, storeFailure(err)
	}
	return n, nil
}

func (m *Manager) run() {
	defer close(m.done)
	pollTimer := m.timing.NewTimer(time.Second)
	cleanupTimer := m.timing.NewTimer(10 * time.Second)
	expiry := m.timing.NewTimer(m.nextExpiry())
	close(m.started)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		poll := pollTimer
		defer func() { poll.Stop() }()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-poll.C():
				poll = m.timing.NewTimer(time.Second)
				m.Poll(m.ctx)
			}
		}
	}()
	go func() {
		defer workers.Done()
		cleanup := cleanupTimer
		defer func() { cleanup.Stop() }()
		for {
			select {
			case <-m.ctx.Done():
				return
			case <-cleanup.C():
				cleanup = m.timing.NewTimer(10 * time.Second)
				if _, err := m.Cleanup(m.ctx); err != nil {
					m.mu.Lock()
					for id, w := range m.watches {
						w.cancel()
						delete(m.watches, id)
					}
					m.mu.Unlock()
				}
			}
		}
	}()
	for {
		select {
		case <-m.ctx.Done():
			expiry.Stop()
			workers.Wait()
			return
		case <-m.wake:
			expiry.Stop()
		case <-expiry.C():
			m.expire()
		}
		expiry = m.timing.NewTimer(m.nextExpiry())
	}
}

// Close cancels local lifetimes synchronously and waits for all owned workers.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	for id, w := range m.watches {
		w.cancel()
		delete(m.watches, id)
	}
	m.mu.Unlock()
	m.stop()
	select {
	case <-m.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
