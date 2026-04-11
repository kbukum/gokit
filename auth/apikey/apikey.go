package apikey

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Key represents an API key's metadata (never stores the plaintext).
type Key struct {
	ID          string     // Unique identifier (typically UUID)
	OwnerID     string     // Owner of this key (user, workspace, org, etc.)
	Name        string     // Human-readable label
	KeyHash     string     // SHA-256 hash of the plaintext key
	KeyPrefix   string     // First 8 chars for safe display (e.g., "sk_live_a1")
	Scopes      []string   // Permissions granted to this key
	IsActive    bool       // Soft-delete flag
	ExpiresAt   *time.Time // Optional hard expiry
	GraceEndsAt *time.Time // Grace period end (set during rotation)
	RotatedByID string     // ID of the replacement key (empty if not rotated)
	LastUsedAt  *time.Time // Last validation timestamp
	CreatedAt   time.Time  // Creation timestamp
}

// IsExpiredPastGrace returns true if the key is expired and beyond its grace period.
func (k *Key) IsExpiredPastGrace() bool {
	now := time.Now()
	if k.GraceEndsAt != nil && now.After(*k.GraceEndsAt) {
		return true
	}
	if k.ExpiresAt != nil && now.After(*k.ExpiresAt) && k.GraceEndsAt == nil {
		return true
	}
	return false
}

// GenerateResult holds the plaintext key (returned once) and its metadata.
type GenerateResult struct {
	PlainKey string // The plaintext key — show to the user once, then discard
	KeyHash  string // SHA-256 hash for storage
	Prefix   string // Display-safe prefix
}

// Generate creates a new random API key with the given prefix.
// The prefix is prepended to 32 hex characters (16 random bytes).
// Example: Generate("sk_live_") → "sk_live_a1b2c3d4e5f6..."
func Generate(prefix string) (*GenerateResult, error) {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("apikey: generate random bytes: %w", err)
	}

	plainKey := prefix + hex.EncodeToString(randomBytes)
	keyHash := Hash(plainKey)

	// Prefix for display: the configured prefix + first few hex chars, up to 8 total
	displayPrefix := plainKey
	if len(displayPrefix) > 8 {
		displayPrefix = displayPrefix[:8]
	}

	return &GenerateResult{
		PlainKey: plainKey,
		KeyHash:  keyHash,
		Prefix:   displayPrefix,
	}, nil
}

// Hash returns the SHA-256 hex digest of a plaintext API key.
func Hash(plainKey string) string {
	h := sha256.Sum256([]byte(plainKey))
	return hex.EncodeToString(h[:])
}

// Validate checks whether a key is usable: active and not expired past grace.
// Returns a descriptive error if the key is not valid.
func Validate(key *Key) error {
	if !key.IsActive {
		return fmt.Errorf("apikey: key is revoked")
	}
	if key.IsExpiredPastGrace() {
		return fmt.Errorf("apikey: key is expired")
	}
	return nil
}
