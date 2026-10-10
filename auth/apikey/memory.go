package apikey

import (
	"context"
	"sync"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

const (
	defaultMemoryCapacity = 1024
	maxMemoryCapacity     = 4096
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

func missingMemoryKey() error { return apperrors.New(apperrors.ErrCodeNotFound, "Key not found") }

func (s *memoryStore) Create(ctx context.Context, key *Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := key.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.admit(key); err != nil {
		return err
	}
	s.insert(key)
	return nil
}

// admit checks uniqueness and capacity; the caller holds the write lock.
func (s *memoryStore) admit(key *Key) error {
	if _, exists := s.keys[key.ID]; exists {
		return apperrors.New(apperrors.ErrCodeAlreadyExists, "Key already exists")
	}
	if _, exists := s.digests[key.KeyDigest]; exists {
		return apperrors.New(apperrors.ErrCodeAlreadyExists, "Key already exists")
	}
	if len(s.keys) >= s.capacity {
		return apperrors.New(apperrors.ErrCodeRateLimited, "API key storage capacity reached").WithReason("API_KEY_CAPACITY")
	}
	return nil
}

func (s *memoryStore) insert(key *Key) {
	s.keys[key.ID] = key.Clone()
	s.digests[key.KeyDigest] = key.ID
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
	if err := next.Validate(); err != nil {
		return err
	}
	s.keys[id] = next
	return nil
}

func (s *memoryStore) UpdateLastUsed(ctx context.Context, id string, usedAt time.Time) error {
	return s.update(ctx, id, func(key *Key) { key.LastUsedAt = &usedAt })
}

func (s *memoryStore) Rotate(ctx context.Context, r Rotation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	old, found := s.keys[r.OldID]
	if !found {
		return missingMemoryKey()
	}
	if !old.rotatable(r.At) {
		return apperrors.Conflict("API key is revoked, expired or already rotated").WithReason("API_KEY_NOT_ROTATABLE")
	}
	if old.ExpiresAt != nil && r.GraceEndsAt.After(*old.ExpiresAt) {
		return apperrors.InvalidInput("apikey.rotation", "Grace must not extend old key expiry")
	}
	if err := s.admit(r.Replacement); err != nil {
		return err
	}
	next := old.Clone()
	next.GraceEndsAt, next.RotatedByID = &r.GraceEndsAt, r.Replacement.ID
	s.keys[r.OldID] = next
	s.insert(r.Replacement)
	return nil
}

func (s *memoryStore) Revoke(ctx context.Context, id string, at time.Time) error {
	return s.update(ctx, id, func(key *Key) {
		if key.RevokedAt == nil {
			key.RevokedAt = &at
		}
	})
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
