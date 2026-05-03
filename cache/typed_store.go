package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kbukum/gokit/provider"
)

// TypedStore provides JSON-serialized typed cache operations.
type TypedStore[C any] struct {
	store     Store
	keyPrefix string
}

// NewTypedStore creates a typed cache store.
func NewTypedStore[C any](store Store, keyPrefix string) *TypedStore[C] {
	return &TypedStore[C]{store: store, keyPrefix: keyPrefix}
}

func (s *TypedStore[C]) fullKey(key string) string {
	if s.keyPrefix == "" {
		return key
	}
	return s.keyPrefix + ":" + key
}

// Load deserializes JSON from cache. It returns (nil, nil) for a miss.
//
//nolint:nilnil // ContextStore uses nil pointer to represent missing state.
func (s *TypedStore[C]) Load(ctx context.Context, key string) (*C, error) {
	raw, ok, err := s.store.Get(ctx, s.fullKey(key))
	if err != nil {
		return nil, fmt.Errorf("typed cache load %q: %w", key, err)
	}
	if !ok {
		return nil, nil
	}
	var val C
	if err := json.Unmarshal(raw, &val); err != nil {
		return nil, fmt.Errorf("typed cache unmarshal %q: %w", key, err)
	}
	return &val, nil
}

// Save serializes val to JSON and stores it with ttl.
func (s *TypedStore[C]) Save(ctx context.Context, key string, val *C, ttl time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("typed cache marshal %q: %w", key, err)
	}
	if err := s.store.Set(ctx, s.fullKey(key), data, ttl); err != nil {
		return fmt.Errorf("typed cache save %q: %w", key, err)
	}
	return nil
}

// Delete removes key.
func (s *TypedStore[C]) Delete(ctx context.Context, key string) error {
	if err := s.store.Delete(ctx, s.fullKey(key)); err != nil {
		return fmt.Errorf("typed cache delete %q: %w", key, err)
	}
	return nil
}

var _ provider.ContextStore[any] = (*TypedStore[any])(nil)
