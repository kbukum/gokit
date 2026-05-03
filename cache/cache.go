package cache

import (
	"context"
	"time"
)

// Store is the backend-neutral cache contract.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// BatchStore is optionally implemented by stores that support efficient batch reads.
type BatchStore interface {
	GetMany(ctx context.Context, keys []string) (map[string][]byte, error)
}

// CloseStore is optionally implemented by stores that hold resources.
type CloseStore interface {
	Close() error
}
