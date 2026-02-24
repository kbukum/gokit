package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kbukum/gokit/provider"
)

// TypedStore provides typed JSON-serialized get/set operations on Redis.
// It implements provider.ContextStore[C] for use with provider.Stateful.
type TypedStore[C any] struct {
	client    *Client
	keyPrefix string
}

// NewTypedStore creates a TypedStore backed by the given Redis client.
// All keys are prefixed with keyPrefix followed by a colon separator.
func NewTypedStore[C any](client *Client, keyPrefix string) *TypedStore[C] {
	return &TypedStore[C]{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

func (s *TypedStore[C]) fullKey(key string) string {
	if s.keyPrefix == "" {
		return key
	}
	return s.keyPrefix + ":" + key
}

// Load deserializes JSON from Redis. Returns (nil, nil) if key doesn't exist.
func (s *TypedStore[C]) Load(ctx context.Context, key string) (*C, error) {
	raw, err := s.client.Get(ctx, s.fullKey(key))
	if err != nil {
		// go-redis returns redis.Nil for missing keys
		if err.Error() == "redis: nil" {
			return nil, nil
		}
		return nil, fmt.Errorf("typed store load %q: %w", key, err)
	}

	var val C
	if err := json.Unmarshal([]byte(raw), &val); err != nil {
		return nil, fmt.Errorf("typed store unmarshal %q: %w", key, err)
	}
	return &val, nil
}

// Save serializes to JSON and stores with TTL. TTL of 0 means no expiration.
func (s *TypedStore[C]) Save(ctx context.Context, key string, val *C, ttl time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("typed store marshal %q: %w", key, err)
	}
	if err := s.client.Set(ctx, s.fullKey(key), string(data), ttl); err != nil {
		return fmt.Errorf("typed store save %q: %w", key, err)
	}
	return nil
}

// Delete removes the key.
func (s *TypedStore[C]) Delete(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, s.fullKey(key)); err != nil {
		return fmt.Errorf("typed store delete %q: %w", key, err)
	}
	return nil
}

// compile-time interface check
var _ provider.ContextStore[any] = (*TypedStore[any])(nil)
