package apikey

import (
	"context"
	"time"
)

// Store is the persistence contract for API keys.
// Consumers implement this with their database (Postgres, Redis, etc.).
type Store interface {
	// Create persists a new API key.
	Create(ctx context.Context, key *Key) error

	// ListByPrefix retrieves candidate keys that share the human-readable prefix
	// (the segment before the first "." in the plaintext key, e.g. "myapp").
	// The result set is used for constant-time digest comparison; callers should
	// enforce a minimum prefix length to keep the candidate set small.
	ListByPrefix(ctx context.Context, keyPrefix string) ([]*Key, error)

	// GetByID retrieves a key by its unique identifier.
	GetByID(ctx context.Context, id string) (*Key, error)

	// UpdateLastUsed sets the LastUsedAt timestamp.
	UpdateLastUsed(ctx context.Context, id string, usedAt time.Time) error

	// SetRotation marks a key as rotated with a grace window.
	SetRotation(ctx context.Context, id string, graceEndsAt time.Time, rotatedByID string) error

	// SetActive enables or disables a key.
	SetActive(ctx context.Context, id string, active bool) error

	// Delete permanently removes a key.
	Delete(ctx context.Context, id string) error
}
