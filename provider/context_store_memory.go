package provider

import (
	"context"
	"sync"
	"time"
)

// MemoryStore is an in-memory ContextStore for testing and development.
// It enforces TTL expiration on Load for testing fidelity.
// Not intended for production use (no persistence, no distributed locking).
type MemoryStore[C any] struct {
	mu    sync.RWMutex
	items map[string]memEntry[C]
}

type memEntry[C any] struct {
	val       *C
	expiresAt time.Time // zero means no expiration
}

// NewMemoryStore creates a new in-memory ContextStore.
func NewMemoryStore[C any]() *MemoryStore[C] {
	return &MemoryStore[C]{
		items: make(map[string]memEntry[C]),
	}
}

// Load retrieves state. Returns (nil, nil) if key doesn't exist or has expired.
func (s *MemoryStore[C]) Load(_ context.Context, key string) (*C, error) {
	s.mu.RLock()
	entry, ok := s.items[key]
	s.mu.RUnlock()

	if !ok {
		return nil, nil
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		s.mu.Lock()
		delete(s.items, key)
		s.mu.Unlock()
		return nil, nil
	}
	return entry.val, nil
}

// Save persists state with optional TTL. TTL of 0 means no expiration.
func (s *MemoryStore[C]) Save(_ context.Context, key string, val *C, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := memEntry[C]{val: val}
	if ttl > 0 {
		entry.expiresAt = time.Now().Add(ttl)
	}
	s.items[key] = entry
	return nil
}

// Delete removes state.
func (s *MemoryStore[C]) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
	return nil
}

// Len returns the number of entries (including expired but not yet cleaned up).
// Useful for test assertions.
func (s *MemoryStore[C]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// compile-time interface check
var _ ContextStore[any] = (*MemoryStore[any])(nil)
