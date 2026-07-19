package apikey

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// DefaultGracePeriod is the duration old keys remain valid after rotation.
const DefaultGracePeriod = 7 * 24 * time.Hour

// RotationConfig configures API key rotation.
type RotationConfig struct {
	GracePeriod time.Duration
	NewKeyID    string
	OwnerID     string
	Name        string
	Prefix      string
	Scopes      []string
	ExpiresAt   *time.Time
}

// RotationResult contains the newly issued key and the persisted replacement record.
type RotationResult struct {
	Issued      GenerateResult
	Record      *Key
	GraceEndsAt time.Time
}

// RotateKey generates a replacement key and moves the old one into a grace window.
func (m *Manager) RotateKey(ctx context.Context, oldKeyID string, cfg RotationConfig) (*RotationResult, error) {
	if cfg.NewKeyID == "" {
		return nil, fmt.Errorf("apikey: NewKeyID is required for rotation")
	}

	oldKey, err := m.store.GetByID(ctx, oldKeyID)
	if err != nil {
		return nil, err
	}
	if validateErr := Validate(oldKey); validateErr != nil {
		return nil, validateErr
	}

	grace := cfg.GracePeriod
	if grace <= 0 {
		grace = DefaultGracePeriod
	}

	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = slices.Clone(oldKey.Scopes)
	}
	ownerID := cfg.OwnerID
	if ownerID == "" {
		ownerID = oldKey.OwnerID
	}
	name := cfg.Name
	if name == "" {
		name = oldKey.Name
	}
	prefix := cfg.Prefix
	if prefix == "" {
		prefix = oldKey.KeyPrefix
	}

	issued, record, err := m.IssueKey(ctx, IssueRequest{
		KeyID:     cfg.NewKeyID,
		OwnerID:   ownerID,
		Name:      name,
		Prefix:    prefix,
		Scopes:    scopes,
		ExpiresAt: cfg.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}

	graceEndsAt := time.Now().Add(grace)
	if err := m.store.SetRotation(ctx, oldKeyID, graceEndsAt, record.ID); err != nil {
		return nil, err
	}

	return &RotationResult{
		Issued:      *issued,
		Record:      record,
		GraceEndsAt: graceEndsAt,
	}, nil
}
