package apikey

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	defaultEntropyBytes = 32
	minEntropyBytes     = 16
	minPepperBytes      = 32
)

// HashingConfig configures API key generation and storage digests.
type HashingConfig struct {
	// Pepper keys the HMAC-SHA-256 digest used for at-rest storage.
	Pepper string

	// EntropyBytes controls how many random bytes are used for the secret body.
	EntropyBytes int
}

// ApplyDefaults applies secure defaults.
func (c *HashingConfig) ApplyDefaults() {
	if c.EntropyBytes == 0 {
		c.EntropyBytes = defaultEntropyBytes
	}
}

// Validate checks that hashing settings satisfy the Group 05 baseline.
func (c *HashingConfig) Validate() error {
	if len([]byte(c.Pepper)) < minPepperBytes {
		return fmt.Errorf("apikey: pepper must be at least %d bytes", minPepperBytes)
	}
	if c.EntropyBytes < minEntropyBytes {
		return fmt.Errorf("apikey: entropy_bytes must be at least %d", minEntropyBytes)
	}
	return nil
}

// Key represents persisted API key metadata (never the plaintext secret).
type Key struct {
	ID          string
	OwnerID     string
	Name        string
	KeyPrefix   string
	KeyDigest   string
	Scopes      []string
	IsActive    bool
	ExpiresAt   *time.Time
	GraceEndsAt *time.Time
	RotatedByID string
	LastUsedAt  *time.Time
	CreatedAt   time.Time
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

// GenerateResult contains one-time API key material returned to callers.
type GenerateResult struct {
	PlainKey  string
	KeyPrefix string
	KeyDigest string
}

// Hasher issues and verifies API keys with a peppered HMAC digest.
type Hasher struct {
	config HashingConfig
	pepper []byte
}

// NewHasher constructs a secure API key hasher.
func NewHasher(cfg HashingConfig) (*Hasher, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Hasher{
		config: cfg,
		pepper: []byte(cfg.Pepper),
	}, nil
}

// Config returns the active hashing configuration.
func (h *Hasher) Config() HashingConfig {
	return h.config
}

// GenerateKey creates a new API key with a validated prefix and peppered digest.
func (h *Hasher) GenerateKey(prefix string) (*GenerateResult, error) {
	cleanedPrefix, err := validatePrefix(prefix)
	if err != nil {
		return nil, err
	}

	randomBytes := make([]byte, h.config.EntropyBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return nil, fmt.Errorf("apikey: generate random bytes: %w", err)
	}

	secret := base64.RawURLEncoding.EncodeToString(randomBytes)
	plainKey := cleanedPrefix + "." + secret
	return &GenerateResult{
		PlainKey:  plainKey,
		KeyPrefix: cleanedPrefix,
		KeyDigest: h.Digest(plainKey),
	}, nil
}

// Digest returns the peppered HMAC-SHA-256 digest for storage.
func (h *Hasher) Digest(plainKey string) string {
	mac := hmac.New(sha256.New, h.pepper)
	_, _ = mac.Write([]byte(plainKey))
	return hex.EncodeToString(mac.Sum(nil))
}

// Compare performs a constant-time comparison against a stored digest.
func (h *Hasher) Compare(plainKey, storedDigest string) bool {
	computed := h.Digest(plainKey)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedDigest)) == 1
}

// SplitKey separates a plaintext key into prefix and secret components.
func SplitKey(plainKey string) (prefix string, secret string, err error) {
	prefix, secret, ok := strings.Cut(plainKey, ".")
	if !ok || prefix == "" || secret == "" {
		return "", "", fmt.Errorf("apikey: invalid key format")
	}
	return prefix, secret, nil
}

// Manager issues, validates, and rotates API keys using prefix-based lookup.
type Manager struct {
	store  Store
	hasher *Hasher
}

// NewManager constructs a Manager from a store and hasher.
func NewManager(store Store, hasher *Hasher) *Manager {
	return &Manager{store: store, hasher: hasher}
}

// IssueKey generates and persists a new API key record.
func (m *Manager) IssueKey(
	ctx context.Context,
	keyID, ownerID, name, prefix string,
	scopes []string,
	expiresAt *time.Time,
) (*GenerateResult, *Key, error) {
	issued, err := m.hasher.GenerateKey(prefix)
	if err != nil {
		return nil, nil, err
	}
	record := &Key{
		ID:        keyID,
		OwnerID:   ownerID,
		Name:      name,
		KeyPrefix: issued.KeyPrefix,
		KeyDigest: issued.KeyDigest,
		Scopes:    slices.Clone(scopes),
		IsActive:  true,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now(),
	}
	if err := m.store.Create(ctx, record); err != nil {
		return nil, nil, err
	}
	return issued, record, nil
}

// ValidateKey resolves a plaintext key via prefix lookup and constant-time digest compare.
func (m *Manager) ValidateKey(ctx context.Context, plainKey string, requiredScopes ...string) (*Key, error) {
	keyPrefix, _, err := SplitKey(plainKey)
	if err != nil {
		return nil, err
	}

	candidates, err := m.store.ListByPrefix(ctx, keyPrefix)
	if err != nil {
		return nil, err
	}

	var matched *Key
	for _, candidate := range candidates {
		digestMatches := m.hasher.Compare(plainKey, candidate.KeyDigest)
		if digestMatches && matched == nil {
			copyKey := *candidate
			copyKey.Scopes = slices.Clone(candidate.Scopes)
			matched = &copyKey
		}
	}

	if matched == nil {
		return nil, fmt.Errorf("apikey: invalid key")
	}
	if err := Validate(matched); err != nil {
		return nil, fmt.Errorf("apikey: invalid key")
	}
	for _, scope := range requiredScopes {
		if !slices.Contains(matched.Scopes, scope) {
			return nil, fmt.Errorf("apikey: insufficient scope")
		}
	}

	usedAt := time.Now()
	if err := m.store.UpdateLastUsed(ctx, matched.ID, usedAt); err != nil {
		return nil, err
	}
	matched.LastUsedAt = &usedAt
	return matched, nil
}

// Validate checks whether a key is active and not expired past its grace period.
func Validate(key *Key) error {
	if !key.IsActive {
		return fmt.Errorf("apikey: key is revoked")
	}
	if key.IsExpiredPastGrace() {
		return fmt.Errorf("apikey: key is expired")
	}
	return nil
}

func validatePrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("apikey: prefix must be non-empty")
	}
	if len(prefix) < 3 {
		return "", fmt.Errorf("apikey: prefix must be at least 3 characters")
	}
	for _, r := range prefix {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return "", fmt.Errorf("apikey: prefix must contain only [A-Za-z0-9_-]")
		}
	}
	return prefix, nil
}
