package apikey

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
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
	// Domain separates credential kinds even when composition shares a pepper. Defaults to "apikey".
	Domain string

	// EntropyBytes controls how many random bytes are used for the secret body.
	EntropyBytes int
	// Random is an injected cryptographic source; defaults to crypto/rand.Reader.
	Random io.Reader
}

// ApplyDefaults applies secure defaults.
func (c *HashingConfig) ApplyDefaults() {
	if c.Domain == "" {
		c.Domain = "apikey"
	}
	if c.EntropyBytes == 0 {
		c.EntropyBytes = defaultEntropyBytes
	}
	if util.IsNil(c.Random) {
		c.Random = rand.Reader
	}
}

// Validate checks that hashing settings satisfy the Group 05 baseline.
func (c *HashingConfig) Validate() error {
	if len([]byte(c.Pepper)) < minPepperBytes {
		return fmt.Errorf("apikey: pepper must be at least %d bytes", minPepperBytes)
	}
	if c.EntropyBytes < minEntropyBytes || c.EntropyBytes > 256 {
		return fmt.Errorf("apikey: entropy_bytes must be at least %d", minEntropyBytes)
	}
	if c.Domain == "" || len(c.Domain) > 64 {
		return fmt.Errorf("apikey: credential domain must contain 1..64 ASCII identifier characters")
	}
	for _, r := range c.Domain {
		allowed := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._/-", r)
		if !allowed {
			return fmt.Errorf("apikey: invalid credential domain")
		}
	}
	return nil
}

// Key represents persisted API key metadata (never the plaintext secret).
type Key struct {
	ID              string
	OwnerID         string
	Name            string
	KeyPrefix       string
	KeyDigest       string
	Scopes          []string
	Kind            auth.Kind
	RestrictionMode auth.RestrictionMode
	Resources       []string
	ExpiresAt       *time.Time
	GraceEndsAt     *time.Time
	RevokedAt       *time.Time
	RotatedByID     string
	LastUsedAt      *time.Time
	CreatedAt       time.Time
}

// Clone returns independent metadata, including restriction slices and optional timestamps.
func (k *Key) Clone() *Key {
	if k == nil {
		return nil
	}
	out := *k
	out.Scopes = slices.Clone(k.Scopes)
	out.Resources = slices.Clone(k.Resources)
	for _, field := range []**time.Time{&out.ExpiresAt, &out.GraceEndsAt, &out.RevokedAt, &out.LastUsedAt} {
		if *field != nil {
			value := **field
			*field = &value
		}
	}
	return &out
}

// ValidAt reports whether the key authenticates at now: it is not revoked, not expired and, after rotation, still
// inside its grace window.
func (k *Key) ValidAt(now time.Time) bool {
	if k == nil || k.RevokedAt != nil {
		return false
	}
	for _, end := range []*time.Time{k.ExpiresAt, k.GraceEndsAt} {
		if end != nil && !now.Before(*end) {
			return false
		}
	}
	return true
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
	mu     sync.Mutex
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

// Config returns the active hashing configuration with sensitive fields redacted.
func (h *Hasher) Config() HashingConfig {
	cfg := h.config
	cfg.Pepper = ""
	cfg.Random = nil
	return cfg
}

// GenerateKey creates a new API key with a validated prefix and peppered digest.
func (h *Hasher) GenerateKey(prefix string) (*GenerateResult, error) {
	cleanedPrefix, err := validatePrefix(prefix)
	if err != nil {
		return nil, apperrors.InvalidInput("prefix", "Invalid key prefix").WithCause(err)
	}

	randomBytes := make([]byte, h.config.EntropyBytes)
	h.mu.Lock()
	_, err = io.ReadFull(h.config.Random, randomBytes)
	h.mu.Unlock()
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("apikey: generate random bytes: %w", err))
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
	return hex.EncodeToString(h.digest(plainKey))
}

func (h *Hasher) digest(plainKey string) []byte {
	mac := hmac.New(sha256.New, h.pepper)
	_, _ = mac.Write([]byte("gokit.credential.v1\x00" + h.config.Domain + "\x00"))
	_, _ = mac.Write([]byte(plainKey))
	return mac.Sum(nil)
}

// Compare performs a constant-time comparison against a stored digest.
func (h *Hasher) Compare(plainKey, storedDigest string) bool {
	computedBytes := h.digest(plainKey)

	decoded, err := hex.DecodeString(storedDigest)
	var stored [sha256.Size]byte
	valid := err == nil && len(decoded) == sha256.Size
	if valid {
		copy(stored[:], decoded)
	}

	matched := subtle.ConstantTimeCompare(computedBytes, stored[:]) == 1
	return valid && matched
}

// SplitKey separates a plaintext key into prefix and secret components.
func SplitKey(plainKey string) (prefix string, secret string, err error) {
	prefix, secret, ok := strings.Cut(plainKey, ".")
	if !ok || prefix == "" || secret == "" {
		return "", "", fmt.Errorf("apikey: invalid key format")
	}
	return prefix, secret, nil
}

func validatePrefix(prefix string) (string, error) {
	if len(prefix) > 128 {
		return "", fmt.Errorf("apikey: prefix must be at most 128 characters")
	}
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
