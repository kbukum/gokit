package stateful

import (
	"context"
	"time"
)

// Store provides typed storage for accumulation. Implementations can use any backend:
// memory, Redis, Postgres, DynamoDB, filesystem, etc.
//
// The Store interface is the pluggability point - users implement this interface
// to use their preferred storage backend.
type Store[V any] interface {
	// Append adds a value to the store. Returns error if operation fails.
	Append(ctx context.Context, value V) error

	// AppendFIFO adds a value and evicts oldest values if total count exceeds maxSize.
	// Returns the evicted values (if any) and an error if operation fails.
	// If maxSize is 0 or negative, behaves like Append (no eviction).
	AppendFIFO(ctx context.Context, value V, maxSize int) (evicted []V, err error)

	// Get returns all values without removing them.
	Get(ctx context.Context) ([]V, error)

	// Flush returns all values and removes them from the store.
	Flush(ctx context.Context) ([]V, error)

	// Size returns the current number of values in the store.
	Size(ctx context.Context) (int, error)

	// Touch updates the last activity time. Used for keep-alive TTL.
	Touch(ctx context.Context) error

	// LastActivity returns the time of last activity (append or touch).
	// Used to check expiration. Returns zero time if never active.
	LastActivity(ctx context.Context) (time.Time, error)

	// Close releases any resources held by the store.
	Close() error
}
