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

	// GetByHash retrieves a key by its SHA-256 hash.
	// Returns an error if not found.
	GetByHash(ctx context.Context, keyHash string) (*Key, error)

	// GetByID retrieves a key by its unique identifier.
	GetByID(ctx context.Context, id string) (*Key, error)

	// UpdateLastUsed sets the LastUsedAt timestamp.
	UpdateLastUsed(ctx context.Context, id string) error

	// SetGracePeriod marks a key as rotated with a grace window.
	SetGracePeriod(ctx context.Context, id string, graceEndsAt time.Time, rotatedByID string) error

	// SetActive enables or disables a key.
	SetActive(ctx context.Context, id string, active bool) error

	// Delete permanently removes a key.
	Delete(ctx context.Context, id string) error
}
