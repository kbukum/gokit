package apikey

import (
	"context"
	"encoding/hex"
	"sync"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
)

const (
	defaultMemoryCapacity = 1024
	maxMemoryCapacity     = 4096
	maxMetadataBytes      = 16 << 10
)

type memoryStore struct {
	mu       sync.RWMutex
	capacity int
	keys     map[string]*Key
	digests  map[string]string
}

// NewMemoryStore creates a bounded, instance-owned default store with a unique digest index.
// Capacity zero selects 1,024 records; positive capacities cannot exceed 4,096.
// Metadata is copied on every boundary. There is no eviction or background worker:
// composition must explicitly Delete records to reclaim capacity.
func NewMemoryStore(capacity int) (Store, error) {
	if capacity < 0 || capacity > maxMemoryCapacity {
		return nil, apperrors.InvalidInput("apikey", "Memory capacity must be from 0 to 4096")
	}
	if capacity == 0 {
		capacity = defaultMemoryCapacity
	}
	return &memoryStore{capacity: capacity, keys: make(map[string]*Key), digests: make(map[string]string)}, nil
}

func validateMemoryKey(key *Key) error {
	if key == nil || key.ID == "" || len(key.ID) > 512 || len(key.OwnerID) > 512 || len(key.Name) > 512 ||
		len(key.RotatedByID) > 512 || len(key.Resources) > 256 || len(key.Scopes) > 256 {
		return apperrors.InvalidInput("apikey", "Invalid key metadata")
	}
	if _, err := validatePrefix(key.KeyPrefix); err != nil {
		return apperrors.InvalidInput("apikey", "Invalid key prefix")
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

func missingMemoryKey() error { return apperrors.New(apperrors.ErrCodeNotFound, "Key not found") }

func (s *memoryStore) Create(ctx context.Context, key *Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateMemoryKey(key); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, exists := s.keys[key.ID]; exists {
		return apperrors.New(apperrors.ErrCodeAlreadyExists, "Key already exists")
	}
	if _, exists := s.digests[key.KeyDigest]; exists {
		return apperrors.New(apperrors.ErrCodeAlreadyExists, "Key already exists")
	}
	if len(s.keys) >= s.capacity {
		return apperrors.New(apperrors.ErrCodeRateLimited, "API key storage capacity reached").WithReason("API_KEY_CAPACITY")
	}
	s.keys[key.ID] = key.Clone()
	s.digests[key.KeyDigest] = key.ID
	return nil
}

func (s *memoryStore) GetByDigest(ctx context.Context, digest string) (*Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, found := s.digests[digest]
	if !found {
		return nil, missingMemoryKey()
	}
	return s.keys[id].Clone(), nil
}

func (s *memoryStore) GetByID(ctx context.Context, id string) (*Key, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, found := s.keys[id]
	if !found {
		return nil, missingMemoryKey()
	}
	return key.Clone(), nil
}

func (s *memoryStore) update(ctx context.Context, id string, apply func(*Key)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	key, found := s.keys[id]
	if !found {
		return missingMemoryKey()
	}
	next := key.Clone()
	apply(next)
	if err := validateMemoryKey(next); err != nil {
		return err
	}
	s.keys[id] = next
	return nil
}

func (s *memoryStore) UpdateLastUsed(ctx context.Context, id string, usedAt time.Time) error {
	return s.update(ctx, id, func(key *Key) { key.LastUsedAt = &usedAt })
}

func (s *memoryStore) SetRotation(ctx context.Context, id string, graceEndsAt time.Time, rotatedByID string) error {
	if len(rotatedByID) > 512 {
		return apperrors.InvalidInput("apikey", "Replacement key ID must not exceed 512 bytes")
	}
	return s.update(ctx, id, func(key *Key) { key.GraceEndsAt = &graceEndsAt; key.RotatedByID = rotatedByID })
}

func (s *memoryStore) SetActive(ctx context.Context, id string, active bool) error {
	return s.update(ctx, id, func(key *Key) { key.IsActive = active })
}

func (s *memoryStore) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	key, found := s.keys[id]
	if !found {
		return missingMemoryKey()
	}
	delete(s.keys, id)
	delete(s.digests, key.KeyDigest)
	return nil
}
