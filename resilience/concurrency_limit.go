package resilience

import (
	"sync"

	apperr "github.com/kbukum/gokit/errors"
)

// ConcurrencyLimit bounds concurrent work overall and per key, without waiting. A caller takes a slot before it knows the key, such as before authentication, then binds the slot to the key once known, so work for one key cannot take every slot. Per-key counts exist only for keys that hold slots, so memory stays bounded by the total.
type ConcurrencyLimit struct {
	total  int
	perKey int

	mu    sync.Mutex
	inUse int
	held  map[string]int
}

// NewConcurrencyLimit allows total concurrent slots, at most perKey of them bound to one key.
func NewConcurrencyLimit(total, perKey int) (*ConcurrencyLimit, error) {
	if total < 1 {
		return nil, apperr.InvalidInput("total", "must be at least 1")
	}
	if perKey < 1 || perKey > total {
		return nil, apperr.InvalidInput("per_key", "must be between 1 and total")
	}
	return &ConcurrencyLimit{total: total, perKey: perKey, held: make(map[string]int)}, nil
}

// TryAcquire takes a slot, or reports false when every slot is in use. The caller must Release the slot.
func (l *ConcurrencyLimit) TryAcquire() (*ConcurrencySlot, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inUse >= l.total {
		return nil, false
	}
	l.inUse++
	return &ConcurrencySlot{limit: l}, true
}

// InUse is the number of slots taken.
func (l *ConcurrencyLimit) InUse() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inUse
}

// Held is the number of slots bound to key.
func (l *ConcurrencyLimit) Held(key string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held[key]
}

// ConcurrencySlot is one slot of a [ConcurrencyLimit]. Its methods are safe for concurrent use.
type ConcurrencySlot struct {
	limit    *ConcurrencyLimit
	key      string
	bound    bool
	released bool
}

// Bind counts the slot against key, or reports false when key already holds its share, the slot is bound, or the slot was released. A refused slot stays usable and still needs Release.
func (s *ConcurrencySlot) Bind(key string) bool {
	l := s.limit
	l.mu.Lock()
	defer l.mu.Unlock()
	if s.bound || s.released || l.held[key] >= l.perKey {
		return false
	}
	l.held[key]++
	s.key, s.bound = key, true
	return true
}

// Release frees the slot and its key binding. Later calls do nothing.
func (s *ConcurrencySlot) Release() {
	l := s.limit
	l.mu.Lock()
	defer l.mu.Unlock()
	if s.released {
		return
	}
	s.released = true
	l.inUse--
	if s.bound {
		l.held[s.key]--
		if l.held[s.key] == 0 {
			delete(l.held, s.key)
		}
	}
}
