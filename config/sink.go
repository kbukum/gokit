package config

import (
	"sort"
	"sync"
)

// ConfigSink is the contract for writable configuration backends. A ConfigSink
// persists or patches configuration values back to a backend — a file, an in-memory
// store, or a future remote backend such as Vault or a Kubernetes secret. Sinks are
// injected explicitly, never via a global registry. Values flow as SecretString
// end-to-end; implementations must never log the plaintext value (only the key may
// appear in diagnostics). Writing the plaintext to the backing store is the sink's
// intended, explicit persistence — not a leak.
type ConfigSink interface {
	// Set stores or replaces the value at key.
	Set(key string, value SecretString) error
	// Remove deletes the value at key. Removing a missing key succeeds (idempotent).
	Remove(key string) error
	// SetMany applies several key/value pairs, failing fast on the first error and
	// leaving earlier writes applied.
	SetMany(entries []ConfigEntry) error
}

// ConfigEntry is one key/value pair applied through ConfigSink.SetMany.
type ConfigEntry struct {
	// Key is the opaque storage key.
	Key string
	// Value is the secret value stored at Key.
	Value SecretString
}

// InMemoryConfigSink is a process-local ConfigSink backed by a map, guarded by a
// mutex. It is useful for tests, defaults, and composing override layers without
// touching disk or a remote backend.
type InMemoryConfigSink struct {
	mu     sync.Mutex
	values map[string]SecretString
	subs   map[chan ConfigChange]struct{}
}

// NewInMemoryConfigSink creates an empty in-memory sink.
func NewInMemoryConfigSink() *InMemoryConfigSink {
	return &InMemoryConfigSink{values: make(map[string]SecretString)}
}

// Set stores or replaces the value at key.
func (s *InMemoryConfigSink) Set(key string, value SecretString) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	s.emit(ConfigChange{Kind: ChangeSet, Key: key})
	return nil
}

// Remove deletes the value at key; removing an absent key is a no-op.
func (s *InMemoryConfigSink) Remove(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	s.emit(ConfigChange{Kind: ChangeRemoved, Key: key})
	return nil
}

// SetMany applies each entry in order, failing fast on the first error.
func (s *InMemoryConfigSink) SetMany(entries []ConfigEntry) error {
	for _, entry := range entries {
		if err := s.Set(entry.Key, entry.Value); err != nil {
			return err
		}
	}
	return nil
}

// Get returns the value stored at key and whether it was present. The returned
// SecretString masks its plaintext; use Value to read it intentionally.
func (s *InMemoryConfigSink) Get(key string) (SecretString, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	return value, ok
}

// Len returns the number of stored keys.
func (s *InMemoryConfigSink) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.values)
}

// IsEmpty reports whether the sink holds no keys.
func (s *InMemoryConfigSink) IsEmpty() bool {
	return s.Len() == 0
}

// Keys returns the stored keys in sorted order.
func (s *InMemoryConfigSink) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.values))
	for key := range s.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
