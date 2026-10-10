package apikey

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

const (
	maxKeyIDBytes  = 512
	maxPlainBytes  = 512
	lookupBudget   = 500 * time.Millisecond
	mutationBudget = 2 * time.Second
)

// Config holds the manager's required dependencies.
type Config struct {
	Store  Store
	Hasher *Hasher
	// Clock supplies issuance, expiry, grace and revocation time.
	Clock util.Clock
}

// Validate reports a missing dependency.
func (c Config) Validate() error {
	switch {
	case util.IsNil(c.Store):
		return apperrors.InvalidInput("store", "API key store is required")
	case c.Hasher == nil:
		return apperrors.InvalidInput("hasher", "API key hasher is required")
	case util.IsNil(c.Clock):
		return apperrors.InvalidInput("clock", "Clock is required")
	}
	return nil
}

// Manager issues, validates, rotates and revokes API keys using indexed protected digest lookup.
type Manager struct {
	store  Store
	hasher *Hasher
	clock  util.Clock
}

// NewManager constructs a Manager from validated dependencies.
func NewManager(cfg Config) (*Manager, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Manager{store: cfg.Store, hasher: cfg.Hasher, clock: cfg.Clock}, nil
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
	ctx, cancel := context.WithTimeout(ctx, mutationBudget)
	defer cancel()
	issued, record, err := m.build(req)
	if err != nil {
		return nil, nil, err
	}
	if err := m.store.Create(ctx, record.Clone()); err != nil {
		return nil, nil, err
	}
	return issued, record, nil
}

// build validates req and generates key material and its record.
func (m *Manager) build(req IssueRequest) (*GenerateResult, *Key, error) {
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
	now := m.clock.Now()
	if req.ExpiresAt != nil && !req.ExpiresAt.After(now) {
		return nil, nil, apperrors.InvalidInput("expires_at", "Expiry must be in the future")
	}
	issued, err := m.hasher.GenerateKey(req.Prefix)
	if err != nil {
		return nil, nil, err
	}
	record := &Key{
		ID: req.KeyID, OwnerID: req.OwnerID, Name: req.Name, KeyPrefix: issued.KeyPrefix, KeyDigest: issued.KeyDigest,
		Scopes: slices.Clone(req.Scopes), Kind: req.Kind, RestrictionMode: req.RestrictionMode, Resources: slices.Clone(req.Resources),
		CreatedAt: now,
	}
	if req.ExpiresAt != nil {
		expires := *req.ExpiresAt
		record.ExpiresAt = &expires
	}
	return issued, record, nil
}

// ValidateKey resolves a plaintext key through one indexed protected digest lookup.
func (m *Manager) ValidateKey(ctx context.Context, plainKey string, requiredScopes ...string) (*Key, error) {
	if len(plainKey) > maxPlainBytes {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}
	keyPrefix, _, err := SplitKey(plainKey)
	if err != nil {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}
	// Reject malformed prefixes before hitting the store (mirrors generation-time validation).
	if _, prefixErr := validatePrefix(keyPrefix); prefixErr != nil {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}
	ctx, cancel := context.WithTimeout(ctx, lookupBudget)
	defer cancel()
	candidate, err := m.store.GetByDigest(ctx, m.hasher.Digest(plainKey))
	if failure := apperrors.FromContext(ctx, "apikey.validate"); failure != nil {
		// A caller that left or a spent budget is not a credential decision.
		return nil, failure
	}
	if err == nil && (candidate == nil || !m.hasher.Compare(plainKey, candidate.KeyDigest)) {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}
	return m.check(candidate, err, requiredScopes)
}

// ValidateKeyID applies the same activity, expiry, grace and scope rules as [Manager.ValidateKey] to a key id. The
// caller proves possession some other way, for example a delegation issued after ValidateKey succeeded, so it is for
// trusted callers only.
func (m *Manager) ValidateKeyID(ctx context.Context, id string, requiredScopes ...string) (*Key, error) {
	if id == "" || len(id) > maxKeyIDBytes {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}
	ctx, cancel := context.WithTimeout(ctx, lookupBudget)
	defer cancel()
	candidate, err := m.store.GetByID(ctx, id)
	if failure := apperrors.FromContext(ctx, "apikey.validate"); failure != nil {
		// A caller that left or a spent budget is not a credential decision.
		return nil, failure
	}
	if err == nil && (candidate == nil || candidate.ID != id) {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}
	return m.check(candidate, err, requiredScopes)
}

func (m *Manager) check(candidate *Key, err error, requiredScopes []string) (*Key, error) {
	if err != nil {
		if app, ok := apperrors.AsAppError(err); ok && app.Code == apperrors.ErrCodeNotFound {
			return nil, auth.Failure("INVALID_CREDENTIAL")
		}
		return nil, storeUnavailable(err)
	}
	if !candidate.ValidAt(m.clock.Now()) {
		return nil, auth.Failure("INVALID_CREDENTIAL")
	}
	for _, scope := range requiredScopes {
		if !slices.Contains(candidate.Scopes, scope) {
			return nil, apperrors.New(apperrors.ErrCodeForbidden, "Permission denied").WithReason("CREDENTIAL_CEILING")
		}
	}
	return candidate.Clone(), nil
}

// RevokeKey ends a key at once, including any remaining rotation grace. Revocation is one-way and idempotent.
func (m *Manager) RevokeKey(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, mutationBudget)
	defer cancel()
	return m.store.Revoke(ctx, id, m.clock.Now())
}

func storeUnavailable(err error) error {
	return apperrors.New(apperrors.ErrCodeServiceUnavailable, "Authentication store unavailable").WithReason("AUTH_STORE_UNAVAILABLE").WithCause(err)
}

// Authenticate maps persisted metadata to the shared principal, whose Reference is the key id. Header keys need no
// CSRF.
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
	p := auth.Principal{Subject: key.OwnerID, Kind: key.Kind, Credential: auth.APIKey, Reference: key.ID, Restrictions: auth.Restrictions{Mode: key.RestrictionMode, Resources: slices.Clone(key.Resources), Scopes: slices.Clone(key.Scopes)}}
	for _, end := range []*time.Time{key.ExpiresAt, key.GraceEndsAt} {
		if end != nil && (p.ExpiresAt.IsZero() || end.Before(p.ExpiresAt)) {
			p.ExpiresAt = *end
		}
	}
	if err := p.Validate(); err != nil {
		return auth.Principal{}, err
	}
	return p, nil
}
