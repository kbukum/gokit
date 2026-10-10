package apikey

import (
	"encoding/hex"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
)

const maxMetadataBytes = 16 << 10

// Validate enforces the common bounded persistence contract before any adapter writes a record.
func (key *Key) Validate() error {
	if key == nil || key.ID == "" || len(key.ID) > 512 || len(key.OwnerID) > 512 || len(key.Name) > 512 ||
		len(key.RotatedByID) > 512 || len(key.Resources) > 256 || len(key.Scopes) > 256 {
		return apperrors.InvalidInput("apikey", "Invalid key metadata")
	}
	if _, err := validatePrefix(key.KeyPrefix); err != nil {
		return apperrors.InvalidInput("apikey", "Invalid key prefix").WithCause(err)
	}
	digest, err := hex.DecodeString(key.KeyDigest)
	if err != nil || len(digest) != 32 || hex.EncodeToString(digest) != key.KeyDigest {
		return apperrors.InvalidInput("apikey", "A protected canonical key digest is required")
	}
	p := auth.Principal{Subject: key.OwnerID, Kind: key.Kind, Credential: auth.APIKey, Reference: key.KeyDigest, Restrictions: auth.Restrictions{Mode: key.RestrictionMode, Resources: key.Resources, Scopes: key.Scopes}}
	if err := p.Validate(); err != nil {
		return apperrors.InvalidInput("apikey", "Invalid identity or restrictions").WithCause(err)
	}
	size := len(key.ID) + len(key.OwnerID) + len(key.Name) + len(key.KeyPrefix) + len(key.KeyDigest) + len(key.RotatedByID)
	for _, values := range [][]string{key.Resources, key.Scopes} {
		for _, value := range values {
			if len(value) > 512 {
				return apperrors.InvalidInput("apikey", "Restriction values must not exceed 512 bytes")
			}
			size += len(value)
		}
	}
	if size > maxMetadataBytes {
		return apperrors.InvalidInput("apikey", "Key metadata must not exceed 16 KiB")
	}
	return nil
}

// Validate checks a rotation request before its transactional compare-and-swap. Adapters must also check the old record's eligibility and expiry inside that transaction.
func (r Rotation) Validate() error {
	if r.OldID == "" || len(r.OldID) > 512 || r.At.IsZero() || r.GraceEndsAt.Before(r.At) || r.GraceEndsAt.After(r.At.Add(MaxGrace)) {
		return apperrors.InvalidInput("apikey.rotation", "Rotation requires an id, check time and bounded grace")
	}
	if err := r.Replacement.Validate(); err != nil {
		return err
	}
	if r.Replacement.ID == r.OldID || r.Replacement.RevokedAt != nil || r.Replacement.RotatedByID != "" || r.Replacement.GraceEndsAt != nil {
		return apperrors.InvalidInput("apikey.rotation", "Replacement must be a fresh independent key")
	}
	return nil
}
