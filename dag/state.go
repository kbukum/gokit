package dag

import (
	"fmt"
	"sync"
)

// State is a thread-safe key-value store for passing data between nodes.
type State struct {
	mu   sync.RWMutex
	data map[string]any
}

// NewState creates a new empty State.
func NewState() *State {
	return &State{data: make(map[string]any)}
}

// Get retrieves a value by key. Returns false if the key does not exist.
func (s *State) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

// Set stores a value by key.
func (s *State) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
}

// Port is a compile-time typed accessor for State.
// It prevents type mismatches between nodes at compile time.
type Port[T any] struct {
	Key string
}

// Read retrieves a typed value from state using a Port.
// Returns an error if the key is missing or the type doesn't match.
func Read[T any](state *State, port Port[T]) (T, error) {
	var zero T
	raw, ok := state.Get(port.Key)
	if !ok {
		return zero, fmt.Errorf("dag: state key %q not found", port.Key)
	}
	val, ok := raw.(T)
	if !ok {
		return zero, fmt.Errorf("dag: state key %q: expected %T, got %T", port.Key, zero, raw)
	}
	return val, nil
}

// Write stores a typed value into state using a Port.
func Write[T any](state *State, port Port[T], value T) {
	state.Set(port.Key, value)
}
