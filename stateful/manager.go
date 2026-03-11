package stateful

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Manager manages multiple named accumulators for multi-tenant use cases.
// Typical key types: string (user ID, session ID), int64 (entity ID), etc.
type Manager[K comparable, V any] struct {
	accumulators sync.Map // K -> *Accumulator[V]
	factory      func(K) *Accumulator[V]
	ttl          time.Duration
	cleanupTick  *time.Ticker
	stopCleanup  chan struct{}
	once         sync.Once
}

// NewManager creates a manager that uses the factory function to create
// accumulators on demand. The TTL is used for automatic cleanup of expired
// accumulators (runs every TTL/4).
//
// The factory function is called once per key when GetOrCreate is called
// or when Append is called for a new key.
func NewManager[K comparable, V any](
	factory func(K) *Accumulator[V],
	ttl time.Duration,
) *Manager[K, V] {
	mgr := &Manager[K, V]{
		factory:     factory,
		ttl:         ttl,
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup ticker if TTL is set
	if ttl > 0 {
		cleanupInterval := ttl / 4
		if cleanupInterval < time.Minute {
			cleanupInterval = time.Minute
		}
		mgr.cleanupTick = time.NewTicker(cleanupInterval)
		go mgr.cleanupLoop()
	}

	return mgr
}

// Get retrieves an accumulator by key. Returns nil if not found.
func (m *Manager[K, V]) Get(key K) *Accumulator[V] {
	val, ok := m.accumulators.Load(key)
	if !ok {
		return nil
	}
	return val.(*Accumulator[V])
}

// GetOrCreate retrieves an accumulator by key, creating it if it doesn't exist.
func (m *Manager[K, V]) GetOrCreate(key K) *Accumulator[V] {
	// Fast path: already exists
	if acc := m.Get(key); acc != nil {
		return acc
	}

	// Create new accumulator
	acc := m.factory(key)

	// Try to store it
	actual, loaded := m.accumulators.LoadOrStore(key, acc)
	if loaded {
		// Another goroutine created it first, close ours and use theirs
		_ = acc.Close()
		return actual.(*Accumulator[V])
	}

	return acc
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
	val, loaded := m.accumulators.LoadAndDelete(key)
	if !loaded {
		return false
	}
	acc := val.(*Accumulator[V])
	_ = acc.Close()
	return true
}

// List returns all currently managed keys.
func (m *Manager[K, V]) List() []K {
	var keys []K
	m.accumulators.Range(func(key, value interface{}) bool {
		keys = append(keys, key.(K))
		return true
	})
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
// This is called automatically if TTL is set, but can also be called manually.
func (m *Manager[K, V]) Cleanup() int {
	ctx := context.Background()
	count := 0

	m.accumulators.Range(func(key, value interface{}) bool {
		acc := value.(*Accumulator[V])
		if acc.IsExpired(ctx) {
			k := key.(K)
			if m.Delete(k) {
				count++
				// Call OnExpire handler if set
				if acc.config.OnExpire != nil {
					acc.config.OnExpire(ctx, fmt.Sprintf("%v", k))
				}
			}
		}
		return true
	})

	return count
}

// Close stops the cleanup ticker and closes all accumulators.
// The manager should not be used after calling Close.
func (m *Manager[K, V]) Close() error {
	m.once.Do(func() {
		// Stop cleanup loop
		if m.cleanupTick != nil {
			m.cleanupTick.Stop()
			close(m.stopCleanup)
		}

		// Close all accumulators
		m.accumulators.Range(func(key, value interface{}) bool {
			acc := value.(*Accumulator[V])
			_ = acc.Close()
			return true
		})
	})
	return nil
}

// cleanupLoop runs the automatic cleanup ticker.
func (m *Manager[K, V]) cleanupLoop() {
	for {
		select {
		case <-m.cleanupTick.C:
			m.Cleanup()
		case <-m.stopCleanup:
			return
		}
	}
}
