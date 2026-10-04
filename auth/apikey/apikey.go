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
	"io"
	"net/http"
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
	IsActive        bool
	ExpiresAt       *time.Time
	GraceEndsAt     *time.Time
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
	for _, field := range []**time.Time{&out.ExpiresAt, &out.GraceEndsAt, &out.LastUsedAt} {
		if *field != nil {
			value := **field
			*field = &value
		}
	}
	return &out
}

// IsExpiredPastGrace returns true if the key is expired and beyond its grace period.
func (k *Key) IsExpiredPastGrace() bool {
	return k.expired(time.Now())
}

func (k *Key) expired(now time.Time) bool {
	if k.GraceEndsAt != nil && !now.Before(*k.GraceEndsAt) {
		return true
	}
	if k.ExpiresAt != nil && !now.Before(*k.ExpiresAt) && k.GraceEndsAt == nil {
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
		return nil, err
	}

	randomBytes := make([]byte, h.config.EntropyBytes)
	h.mu.Lock()
	_, err = io.ReadFull(h.config.Random, randomBytes)
	h.mu.Unlock()
	if err != nil {
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

// Manager issues, validates, and rotates API keys using indexed protected digest lookup.
type Manager struct {
	store  Store
	hasher *Hasher
	clock  util.Clock
}

// ManagerOption configures injected manager concerns.
type ManagerOption func(*Manager)

// WithClock injects expiry and audit time.
func WithClock(clock util.Clock) ManagerOption {
	return func(m *Manager) {
		if clock != nil {
			m.clock = clock
		}
	}
}

// NewManager constructs a Manager from a store and hasher.
func NewManager(store Store, hasher *Hasher, opts ...ManagerOption) *Manager {
	m := &Manager{store: store, hasher: hasher, clock: util.SystemClock{}}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// IssueRequest describes a new API key to issue.
type IssueRequest struct {
	KeyID           string
	OwnerID         string
	Name            string
	Prefix          string
	Scopes          []string
	Kind            auth.Kind
	RestrictionMode auth.RestrictionMode
	Resources       []string
	ExpiresAt       *time.Time
}

// IssueKey generates and persists a new API key record.
func (m *Manager) IssueKey(ctx context.Context, req IssueRequest) (*GenerateResult, *Key, error) {
	if util.IsNil(m.store) || m.hasher == nil {
		return nil, nil, apperrors.InvalidInput("apikey", "Store and hasher are required")
	}
	if req.Kind == "" {
		req.Kind = auth.User
	}
	if req.RestrictionMode == "" {
		req.RestrictionMode = auth.Unrestricted
		if len(req.Scopes) != 0 || len(req.Resources) != 0 {
			req.RestrictionMode = auth.Restricted
		}
	}
	if req.OwnerID == "" || req.KeyID == "" || (req.Kind != auth.User && req.Kind != auth.Service) {
		return nil, nil, auth.Failure("INVALID_IDENTITY")
	}
	if err := (auth.Restrictions{Mode: req.RestrictionMode, Resources: req.Resources, Scopes: req.Scopes}).Validate(); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	issued, err := m.hasher.GenerateKey(req.Prefix)
	if err != nil {
		return nil, nil, err
	}
	record := &Key{
		ID:        req.KeyID,
		OwnerID:   req.OwnerID,
		Name:      req.Name,
		KeyPrefix: issued.KeyPrefix,
		KeyDigest: issued.KeyDigest,
		Scopes:    slices.Clone(req.Scopes),
		Kind:      req.Kind, RestrictionMode: req.RestrictionMode, Resources: slices.Clone(req.Resources),
		IsActive:  true,
		ExpiresAt: req.ExpiresAt,
		CreatedAt: m.clock.Now(),
	}
	record = record.Clone()
	if err := m.store.Create(ctx, record.Clone()); err != nil {
		return nil, nil, err
	}
	return issued, record, nil
}

// ValidateKey resolves a plaintext key through one indexed protected digest lookup.
func (m *Manager) ValidateKey(ctx context.Context, plainKey string, requiredScopes ...string) (*Key, error) {
	if len(plainKey) > 512 || util.IsNil(m.store) || m.hasher == nil {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	keyPrefix, _, err := SplitKey(plainKey)
	if err != nil {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}

	// Reject malformed prefixes before hitting the store (mirrors generation-time validation).
	if _, prefixErr := validatePrefix(keyPrefix); prefixErr != nil {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}

	candidate, err := m.store.GetByDigest(ctx, m.hasher.Digest(plainKey))
	if err != nil {
		if app, ok := apperrors.AsAppError(err); ok && app != nil && app.Code == apperrors.ErrCodeNotFound {
			return nil, auth.Failure("INVALID_CREDENTIAL")
		}
		return nil, apperrors.New(apperrors.ErrCodeServiceUnavailable, "Authentication store unavailable").WithReason("AUTH_STORE_UNAVAILABLE").WithCause(err)
	}

	if candidate == nil || !m.hasher.Compare(plainKey, candidate.KeyDigest) || !candidate.IsActive || candidate.expired(m.clock.Now()) {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}
	matched := candidate.Clone()
	for _, scope := range requiredScopes {
		if !slices.Contains(matched.Scopes, scope) {
			return nil, apperrors.New(apperrors.ErrCodeForbidden, "Permission denied").WithReason("CREDENTIAL_CEILING")
		}
	}

	if ctx.Err() != nil {
		return nil, apperrors.Normalize(ctx.Err())
	}
	return matched, nil
}

// Authenticate maps persisted metadata to the shared principal. Header keys need no CSRF.
func (m *Manager) Authenticate(r *http.Request) (auth.Principal, error) {
	presented, err := auth.ParseCredentials(r)
	if err != nil {
		return auth.Principal{}, err
	}
	if presented.Kind != auth.APIKey {
		return auth.Principal{}, auth.Failure("MISSING_CREDENTIAL")
	}
	key, err := m.ValidateKey(r.Context(), presented.Value)
	if err != nil {
		return auth.Principal{}, err
	}
	p := auth.Principal{Subject: key.OwnerID, Kind: key.Kind, Credential: auth.APIKey, Reference: key.KeyDigest, Restrictions: auth.Restrictions{Mode: key.RestrictionMode, Resources: slices.Clone(key.Resources), Scopes: slices.Clone(key.Scopes)}}
	if key.ExpiresAt != nil {
		p.ExpiresAt = *key.ExpiresAt
	}
	if key.GraceEndsAt != nil {
		p.ExpiresAt = *key.GraceEndsAt
	}
	if err := p.Validate(); err != nil {
		return auth.Principal{}, err
	}
	return p, nil
}

// Validate checks whether a key is active and not expired past its grace period.
func Validate(key *Key) error {
	if key == nil || !key.IsActive {
		return fmt.Errorf("apikey: key is revoked")
	}
	if key.IsExpiredPastGrace() {
		return fmt.Errorf("apikey: key is expired")
	}
	return nil
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
