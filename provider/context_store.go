package provider

import (
	"context"
	"time"
)

// ContextStore provides typed state persistence for stateful providers.
// Implementations live in sub-modules (redis, database, etc.) to avoid
// forcing dependencies on the core.
//
// The key is an opaque string — the consumer decides the key schema.
// TTL of 0 means no expiration.
type ContextStore[C any] interface {
	// Load retrieves state. Returns (nil, nil) if key doesn't exist.
	Load(ctx context.Context, key string) (*C, error)
	// Save persists state with optional TTL. TTL of 0 means no expiration.
	Save(ctx context.Context, key string, val *C, ttl time.Duration) error
	// Delete removes state.
	Delete(ctx context.Context, key string) error
}
