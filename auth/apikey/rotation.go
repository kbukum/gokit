package apikey

import (
	"context"
	"fmt"
	"time"
)

// DefaultGracePeriod is the duration old keys remain valid after rotation.
const DefaultGracePeriod = 7 * 24 * time.Hour

// RotationConfig configures key rotation behavior.
type RotationConfig struct {
	// GracePeriod is how long the old key stays valid after rotation.
	// Zero uses DefaultGracePeriod.
	GracePeriod time.Duration

	// Prefix is the key prefix for newly generated keys.
	Prefix string
}

// RotationResult holds the outcome of a key rotation.
type RotationResult struct {
	NewKey      GenerateResult // The newly generated key
	OldKeyID    string         // ID of the rotated (old) key
	GraceEndsAt time.Time      // When the old key stops working
}

// Rotate generates a replacement key and sets a grace period on the old one.
// The old key remains valid until GraceEndsAt.
func Rotate(ctx context.Context, store Store, oldKeyID string, cfg RotationConfig) (*RotationResult, error) {
	oldKey, err := store.GetByID(ctx, oldKeyID)
	if err != nil {
		return nil, fmt.Errorf("apikey: old key not found: %w", err)
	}

	if err := Validate(oldKey); err != nil {
		return nil, fmt.Errorf("apikey: cannot rotate: %w", err)
	}

	newResult, err := Generate(cfg.Prefix)
	if err != nil {
		return nil, fmt.Errorf("apikey: generate replacement: %w", err)
	}

	grace := cfg.GracePeriod
	if grace <= 0 {
		grace = DefaultGracePeriod
	}
	graceEndsAt := time.Now().Add(grace)

	if err := store.SetGracePeriod(ctx, oldKeyID, graceEndsAt, ""); err != nil {
		return nil, fmt.Errorf("apikey: set grace period: %w", err)
	}

	return &RotationResult{
		NewKey:      *newResult,
		OldKeyID:    oldKeyID,
		GraceEndsAt: graceEndsAt,
	}, nil
}
