package stateful

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Manager manages multiple named accumulators for multi-tenant use cases. Typical key types:
// string (user ID, session ID), int64 (entity ID), etc.
type Manager[K comparable, V any] struct {
	mu           sync.RWMutex
	accumulators map[K]*Accumulator[V]
	factory      func(K) *Accumulator[V]
	ttl          time.Duration
	cleanupTick  *time.Ticker
	stopCleanup  chan struct{}
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	inCallback   atomic.Int32
	closed       bool
	lastErr      error
	stopOnce     sync.Once
	teardownOnce sync.Once
}

// NewManager creates a manager that uses the factory function to create accumulators on demand.
// The TTL is used for automatic cleanup of expired accumulators (runs every TTL/4).
//
// The factory function is called once per key when GetOrCreate is called
// or when Append is called for a new key.
func NewManager[K comparable, V any](
	factory func(K) *Accumulator[V],
	ttl time.Duration,
) *Manager[K, V] {
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // G118: cancel is retained on Manager and invoked in Close
	mgr := &Manager[K, V]{
		accumulators: make(map[K]*Accumulator[V]),
		factory:      factory,
		ttl:          ttl,
		stopCleanup:  make(chan struct{}),
		cancel:       cancel,
	}

	// Start cleanup ticker if TTL is set
	if ttl > 0 {
		cleanupInterval := ttl / 4
		if cleanupInterval <= 0 {
			cleanupInterval = ttl
		}
		if cleanupInterval < 10*time.Millisecond {
			cleanupInterval = 10 * time.Millisecond
		}
		mgr.cleanupTick = time.NewTicker(cleanupInterval)
		mgr.wg.Add(1)
		go mgr.cleanupLoop(ctx)
	}

	return mgr
}

// Get retrieves an accumulator by key. Returns nil if not found.
func (m *Manager[K, V]) Get(key K) *Accumulator[V] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.accumulators[key]
}

// GetOrCreate retrieves an accumulator by key, creating it if it doesn't exist.
//
// If the manager has already been closed, GetOrCreate does not store the
// newly-created accumulator: it closes it (recording any close error, surfaced
// through the next Close call) and returns that closed accumulator. Append and
// Touch on a closed accumulator return [ErrAccumulatorClosed] rather than
// silently writing into a released store, so a post-Close [Manager.Append] fails
// instead of succeeding into an untracked accumulator, and it is never orphaned
// inside the manager.
func (m *Manager[K, V]) GetOrCreate(key K) *Accumulator[V] {
	// Fast path: already exists.
	if acc := m.Get(key); acc != nil {
		return acc
	}

	// Create outside the lock, then compare-and-store under the lock so a single
	// accumulator wins for concurrent callers; the loser is closed.
	acc := m.factory(key)

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		m.recordCloseErr(acc.Close())
		return acc
	}
	if existing, ok := m.accumulators[key]; ok {
		m.mu.Unlock()
		m.recordCloseErr(acc.Close())
		return existing
	}
	m.accumulators[key] = acc
	m.mu.Unlock()
	return acc
}

// recordCloseErr joins a non-nil accumulator close error into the manager's
// aggregated close error, which is surfaced through Close. This keeps close
// errors from being silently discarded on the Delete and GetOrCreate-loser
// paths without changing their public signatures.
func (m *Manager[K, V]) recordCloseErr(err error) {
	if err == nil {
		return
	}
	m.mu.Lock()
	m.lastErr = errors.Join(m.lastErr, err)
	m.mu.Unlock()
}

// Append appends a value to the accumulator for the given key.
// Creates the accumulator if it doesn't exist.
func (m *Manager[K, V]) Append(ctx context.Context, key K, value V) error {
	acc := m.GetOrCreate(key)
	return acc.Append(ctx, value)
}

// Flush manually flushes the accumulator for the given key.
// Returns nil values if the key doesn't exist.
func (m *Manager[K, V]) Flush(ctx context.Context, key K) ([]V, error) {
	acc := m.Get(key)
	if acc == nil {
		return nil, nil
	}
	return acc.Flush(ctx)
}

// Delete removes an accumulator by key. Closes the accumulator's resources.
// Returns false if the key didn't exist.
func (m *Manager[K, V]) Delete(key K) bool {
	m.mu.Lock()
	acc, ok := m.accumulators[key]
	if !ok {
		m.mu.Unlock()
		return false
	}
	delete(m.accumulators, key)
	m.mu.Unlock()

	m.recordCloseErr(acc.Close())
	return true
}

// deleteMatching removes key only if it currently maps to acc, closing that
// accumulator. It guards against an ABA race: between a caller snapshotting an
// accumulator and deleting it, another goroutine may delete and recreate a
// different accumulator under the same key. Comparing the live map value with
// the captured pointer under the lock ensures cleanup never removes or closes a
// replacement. Returns false when the key is absent or holds a different
// accumulator.
func (m *Manager[K, V]) deleteMatching(key K, acc *Accumulator[V]) bool {
	m.mu.Lock()
	current, ok := m.accumulators[key]
	if !ok || current != acc {
		m.mu.Unlock()
		return false
	}
	delete(m.accumulators, key)
	m.mu.Unlock()

	m.recordCloseErr(acc.Close())
	return true
}

// List returns all currently managed keys.
func (m *Manager[K, V]) List() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]K, 0, len(m.accumulators))
	for key := range m.accumulators {
		keys = append(keys, key)
	}
	return keys
}

// Size returns the current size of the accumulator for the given key.
// Returns 0 if the key doesn't exist.
func (m *Manager[K, V]) Size(ctx context.Context, key K) (int, error) {
	acc := m.Get(key)
	if acc == nil {
		return 0, nil
	}
	return acc.Size(ctx)
}

// Measure returns the current measurement of the accumulator for the given key.
// Returns 0 if the key doesn't exist.
func (m *Manager[K, V]) Measure(ctx context.Context, key K) (int, error) {
	acc := m.Get(key)
	if acc == nil {
		return 0, nil
	}
	return acc.Measure(ctx)
}

// Cleanup removes expired accumulators. Returns the number of accumulators removed.
// This is called automatically if TTL is set, but can also be called manually;
// a manual call evaluates expiration against context.Background rather than the
// manager's lifecycle context.
func (m *Manager[K, V]) Cleanup() int {
	return m.cleanup(context.Background())
}

// cleanup removes expired accumulators using ctx for expiration checks and
// OnExpire callbacks. The ticker-driven caller passes the manager lifecycle
// context so shutdown cancels in-flight store work.
func (m *Manager[K, V]) cleanup(ctx context.Context) int {
	// Snapshot the current keys/accumulators under the read lock, then evaluate
	// expiration and delete outside it to avoid holding the lock across Delete.
	m.mu.RLock()
	candidates := make(map[K]*Accumulator[V], len(m.accumulators))
	for key, acc := range m.accumulators {
		candidates[key] = acc
	}
	m.mu.RUnlock()

	count := 0
	for key, acc := range candidates {
		if !acc.IsExpired(ctx) {
			continue
		}
		// Compare-and-delete against the captured pointer so a key that was
		// concurrently deleted and recreated is not clobbered.
		if m.deleteMatching(key, acc) {
			count++
			if acc.config.OnExpire != nil {
				// Mark the expiration callback as in-flight so a reentrant
				// Close invoked from within it does not join this goroutine
				// (which would self-deadlock).
				m.inCallback.Add(1)
				acc.config.OnExpire(ctx, fmt.Sprintf("%v", key))
				m.inCallback.Add(-1)
			}
		}
	}

	return count
}

// Close stops the cleanup ticker, drains the cleanup goroutine, and closes all
// accumulators. Any errors returned by accumulator Close calls (including those
// captured earlier on the Delete and GetOrCreate-loser paths) are aggregated
// with errors.Join and returned. Close is idempotent; the manager should not be
// used after calling Close.
//
// Close joins the cleanup goroutine so none outlives it, except when Close is
// invoked reentrantly from within an OnExpire callback running on that
// goroutine — joining there would self-deadlock. In that reentrant case the
// cleanup goroutine performs teardown as it unwinds, and Close returns without
// joining itself.
func (m *Manager[K, V]) Close() error {
	// Phase 1: signal shutdown. Idempotent and never blocks, so it is safe from
	// inside an expiration callback. This unblocks the cleanup goroutine.
	m.stopOnce.Do(func() {
		if m.cleanupTick != nil {
			m.cleanupTick.Stop()
			close(m.stopCleanup)
		}
		m.cancel()
		m.mu.Lock()
		m.closed = true
		m.mu.Unlock()
	})

	// Phase 2: join the cleanup goroutine unless an expiration callback is in
	// flight, which may be this very goroutine calling Close reentrantly.
	if m.inCallback.Load() == 0 {
		m.wg.Wait()
	}

	// Detach and close every accumulator exactly once. The cleanup goroutine
	// also runs this as it unwinds, so teardownOnce makes the winner idempotent.
	m.teardown()

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastErr
}

// teardown detaches the accumulators under the lock and closes each exactly
// once, joining every close error into lastErr. It is safe to call from Close
// or from the cleanup goroutine as it unwinds; teardownOnce elects a single
// winner.
func (m *Manager[K, V]) teardown() {
	m.teardownOnce.Do(func() {
		m.mu.Lock()
		accs := m.accumulators
		m.accumulators = make(map[K]*Accumulator[V])
		m.closed = true
		m.mu.Unlock()

		var closeErr error
		for _, acc := range accs {
			closeErr = errors.Join(closeErr, acc.Close())
		}

		// Join (not overwrite) so close errors recorded concurrently by an
		// in-flight Delete or post-Close GetOrCreate loser are not lost.
		m.mu.Lock()
		m.lastErr = errors.Join(m.lastErr, closeErr)
		m.mu.Unlock()
	})
}

// cleanupLoop runs the automatic cleanup ticker until the manager is closed,
// then tears down remaining accumulators as it exits so a reentrant Close
// (which cannot join this goroutine) still leaves nothing unclosed. ctx is the
// manager lifecycle context; its cancellation stops the loop and cancels
// in-flight cleanup work.
func (m *Manager[K, V]) cleanupLoop(ctx context.Context) {
	defer m.wg.Done()
	defer m.teardown()
	for {
		select {
		case <-m.cleanupTick.C:
			m.cleanup(ctx)
		case <-ctx.Done():
			return
		case <-m.stopCleanup:
			return
		}
	}
}
