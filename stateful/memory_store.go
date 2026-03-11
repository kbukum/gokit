package stateful

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-memory implementation of Store[V].
// Fast and simple, but not durable. Data is lost on restart.
// Thread-safe for concurrent operations.
type MemoryStore[V any] struct {
	values       []V
	lastActivity time.Time
	mu           sync.RWMutex
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore[V any]() *MemoryStore[V] {
	return &MemoryStore[V]{
		values:       make([]V, 0),
		lastActivity: time.Now(),
	}
}

func (s *MemoryStore[V]) Append(_ context.Context, value V) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.values = append(s.values, value)
	s.lastActivity = time.Now()
	return nil
}

func (s *MemoryStore[V]) AppendFIFO(_ context.Context, value V, maxSize int) ([]V, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if maxSize <= 0 {
		// No limit, just append
		s.values = append(s.values, value)
		s.lastActivity = time.Now()
		return nil, nil
	}

	// Append first
	s.values = append(s.values, value)
	s.lastActivity = time.Now()

	// Check if we exceeded max size
	if len(s.values) <= maxSize {
		return nil, nil
	}

	// Evict oldest (FIFO)
	toEvict := len(s.values) - maxSize
	evicted := make([]V, toEvict)
	copy(evicted, s.values[:toEvict])
	s.values = s.values[toEvict:]

	return evicted, nil
}

func (s *MemoryStore[V]) Get(_ context.Context) ([]V, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make([]V, len(s.values))
	copy(result, s.values)
	return result, nil
}

func (s *MemoryStore[V]) Flush(_ context.Context) ([]V, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.values
	s.values = make([]V, 0)
	s.lastActivity = time.Now()
	return result, nil
}

func (s *MemoryStore[V]) Size(_ context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.values), nil
}

func (s *MemoryStore[V]) Touch(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActivity = time.Now()
	return nil
}

func (s *MemoryStore[V]) LastActivity(_ context.Context) (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastActivity, nil
}

func (s *MemoryStore[V]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = nil
	return nil
}

// Compile-time check that MemoryStore implements Store
var _ Store[any] = (*MemoryStore[any])(nil)
