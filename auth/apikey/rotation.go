package apikey

import (
	"context"
	"slices"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

// MaxGrace bounds how long a rotated key keeps validating.
const MaxGrace = 30 * 24 * time.Hour

// RotateRequest describes the replacement for a rotated key. The replacement keeps the old key's owner, name, prefix,
// kind and restrictions; issue a new key to change them.
type RotateRequest struct {
	NewKeyID string
	// Grace is how long the old key keeps validating; zero ends it at once. It is capped at the old key's expiry and
	// must not exceed MaxGrace.
	Grace time.Duration
	// ExpiresAt is the replacement's expiry; nil means none.
	ExpiresAt *time.Time
}

// RotationResult contains the newly issued key and the persisted replacement record.
type RotationResult struct {
	Issued      GenerateResult
	Record      *Key
	GraceEndsAt time.Time
}

// RotateKey replaces a valid, unrotated key in one atomic store operation, so it cannot race revocation or another
// rotation. A key that is revoked, expired or already rotated returns CONFLICT.
func (m *Manager) RotateKey(ctx context.Context, oldKeyID string, req RotateRequest) (*RotationResult, error) {
	if req.NewKeyID == "" {
		return nil, apperrors.InvalidInput("new_key_id", "Replacement key id is required")
	}
	if req.Grace < 0 || req.Grace > MaxGrace {
		return nil, apperrors.InvalidInput("grace", "Grace must be from zero to 30 days")
	}
	ctx, cancel := context.WithTimeout(ctx, mutationBudget)
	defer cancel()
	old, err := m.store.GetByID(ctx, oldKeyID)
	if err != nil {
		return nil, err
	}
	now := m.clock.Now()
	if !old.rotatable(now) {
		return nil, apperrors.Conflict("API key is revoked, expired or already rotated").WithReason("API_KEY_NOT_ROTATABLE")
	}
	issued, record, err := m.build(IssueRequest{
		KeyID: req.NewKeyID, OwnerID: old.OwnerID, Name: old.Name, Prefix: old.KeyPrefix, Scopes: slices.Clone(old.Scopes),
		Kind: old.Kind, RestrictionMode: old.RestrictionMode, Resources: slices.Clone(old.Resources), ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	graceEndsAt := now.Add(req.Grace)
	if old.ExpiresAt != nil && old.ExpiresAt.Before(graceEndsAt) {
		graceEndsAt = *old.ExpiresAt
	}
	if err := m.store.Rotate(ctx, Rotation{OldID: oldKeyID, Replacement: record.Clone(), GraceEndsAt: graceEndsAt, At: now}); err != nil {
		return nil, err
	}
	return &RotationResult{Issued: *issued, Record: record, GraceEndsAt: graceEndsAt}, nil
}
