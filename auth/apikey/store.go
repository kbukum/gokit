package apikey

import (
	"context"
	"time"
)

// Store is the persistence contract for API keys. Every implementation must pass [apikeytest.Run].
//
// Missing keys return an AppError with NOT_FOUND; duplicate ids or digests return ALREADY_EXISTS; infrastructure
// failures must stay distinct from both. Records are copied across the boundary.
type Store interface {
	// Create persists a new API key.
	Create(ctx context.Context, key *Key) error

	// GetByDigest uses a unique indexed protected digest, never an unbounded prefix scan.
	GetByDigest(ctx context.Context, digest string) (*Key, error)

	// GetByID retrieves a key by its unique identifier.
	GetByID(ctx context.Context, id string) (*Key, error)

	// UpdateLastUsed sets the LastUsedAt timestamp.
	UpdateLastUsed(ctx context.Context, id string, usedAt time.Time) error

	// Rotate performs r as one compare-and-swap: when the old key is unrevoked, not yet rotated and valid at r.At,
	// it creates r.Replacement and records r.GraceEndsAt and the replacement id on the old key. Otherwise it changes
	// nothing and returns CONFLICT, or the error Create would return for the replacement.
	Rotate(ctx context.Context, r Rotation) error

	// Revoke records at as the key's revocation time. Revocation is one-way; revoking a revoked key keeps the first
	// time and succeeds.
	Revoke(ctx context.Context, id string, at time.Time) error

	// Delete permanently removes a key.
	Delete(ctx context.Context, id string) error
}

// Rotation is one atomic replacement of an API key.
type Rotation struct {
	OldID       string
	Replacement *Key
	// GraceEndsAt is when the old key stops validating. It is not after the old key's expiry.
	GraceEndsAt time.Time
	// At is the manager's current time, against which the old key must still be valid.
	At time.Time
}

// rotatable reports whether old may be replaced at the given time.
func (k *Key) rotatable(now time.Time) bool { return k.ValidAt(now) && k.RotatedByID == "" }
