package apikey

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func testHasher(t *testing.T) *Hasher {
	t.Helper()
	hasher, err := NewHasher(HashingConfig{Pepper: strings.Repeat("p", 32)})
	if err != nil {
		t.Fatalf("NewHasher: %v", err)
	}
	return hasher
}

func TestHashingConfigValidate(t *testing.T) {
	t.Parallel()

	if _, err := NewHasher(HashingConfig{Pepper: "short"}); err == nil {
		t.Fatal("expected short pepper to fail")
	}
	if _, err := NewHasher(HashingConfig{Pepper: strings.Repeat("p", 32), EntropyBytes: 8}); err == nil {
		t.Fatal("expected short entropy to fail")
	}
}

func TestHasherGenerateAndCompare(t *testing.T) {
	t.Parallel()

	hasher := testHasher(t)
	issued, err := hasher.GenerateKey("sk_live")
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if issued.KeyPrefix != "sk_live" {
		t.Fatalf("KeyPrefix = %q, want sk_live", issued.KeyPrefix)
	}
	if !strings.HasPrefix(issued.PlainKey, "sk_live.") {
		t.Fatalf("PlainKey = %q, want sk_live prefix", issued.PlainKey)
	}
	if !hasher.Compare(issued.PlainKey, issued.KeyDigest) {
		t.Fatal("expected digest comparison to succeed")
	}
	if hasher.Compare(issued.PlainKey+"x", issued.KeyDigest) {
		t.Fatal("expected modified key to fail comparison")
	}
}

func TestSplitKey(t *testing.T) {
	t.Parallel()

	prefix, secret, err := SplitKey("sk.secret")
	if err != nil {
		t.Fatalf("SplitKey: %v", err)
	}
	if prefix != "sk" || secret != "secret" {
		t.Fatalf("SplitKey returned %q %q", prefix, secret)
	}
	if _, _, err := SplitKey("malformed"); err == nil {
		t.Fatal("expected malformed key to fail")
	}
}

type memStore struct {
	mu      sync.Mutex
	byID    map[string]*Key
	listErr error
}

func newMemStore() *memStore {
	return &memStore{byID: map[string]*Key{}}
}

func (s *memStore) Create(_ context.Context, key *Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyKey := *key
	copyKey.Scopes = slices.Clone(key.Scopes)
	s.byID[key.ID] = &copyKey
	return nil
}

func (s *memStore) ListByPrefix(_ context.Context, keyPrefix string) ([]*Key, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]*Key, 0)
	for _, record := range s.byID {
		if record.KeyPrefix == keyPrefix {
			copyKey := *record
			copyKey.Scopes = slices.Clone(record.Scopes)
			keys = append(keys, &copyKey)
		}
	}
	return keys, nil
}

func (s *memStore) GetByID(_ context.Context, id string) (*Key, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	copyKey := *key
	copyKey.Scopes = slices.Clone(key.Scopes)
	return &copyKey, nil
}

func (s *memStore) UpdateLastUsed(_ context.Context, id string, usedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[id].LastUsedAt = &usedAt
	return nil
}

func (s *memStore) SetRotation(_ context.Context, id string, graceEndsAt time.Time, rotatedByID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[id].GraceEndsAt = &graceEndsAt
	s.byID[id].RotatedByID = rotatedByID
	return nil
}

func (s *memStore) SetActive(_ context.Context, id string, active bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byID[id].IsActive = active
	return nil
}

func (s *memStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, id)
	return nil
}

func TestManagerIssueValidateAndRotate(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	manager := NewManager(store, testHasher(t))

	issued, record, err := manager.IssueKey(context.Background(), "key-1", "user-1", "primary", "pkg", []string{"read"}, nil)
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}
	if record.KeyDigest != issued.KeyDigest {
		t.Fatal("record digest mismatch")
	}

	validated, err := manager.ValidateKey(context.Background(), issued.PlainKey, "read")
	if err != nil {
		t.Fatalf("ValidateKey: %v", err)
	}
	if validated.OwnerID != "user-1" || validated.LastUsedAt == nil {
		t.Fatalf("validated = %+v", validated)
	}
	if _, validateErr := manager.ValidateKey(context.Background(), issued.PlainKey, "write"); validateErr == nil {
		t.Fatal("expected scope escalation to fail")
	}

	rotation, err := manager.RotateKey(context.Background(), "key-1", RotationConfig{
		NewKeyID: "key-2",
		OwnerID:  "user-1",
		Name:     "secondary",
		Prefix:   "pkg",
	})
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	if rotation.Record.ID != "key-2" {
		t.Fatalf("rotated record id = %q", rotation.Record.ID)
	}
	original, _ := store.GetByID(context.Background(), "key-1")
	if original.RotatedByID != "key-2" || original.GraceEndsAt == nil {
		t.Fatalf("original rotation not persisted: %+v", original)
	}
}

func TestManagerValidateRejectsExpiredKey(t *testing.T) {
	t.Parallel()

	store := newMemStore()
	manager := NewManager(store, testHasher(t))
	issued, _, err := manager.IssueKey(context.Background(), "key-1", "user-1", "expired", "pkg", nil, nil)
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}

	past := time.Now().Add(-time.Hour)
	store.byID["key-1"].ExpiresAt = &past

	if _, err := manager.ValidateKey(context.Background(), issued.PlainKey); err == nil {
		t.Fatal("expected expired key to fail")
	}
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	manager := NewManager(newMemStore(), testHasher(t))
	issued, _, err := manager.IssueKey(context.Background(), "key-1", "user-1", "primary", "pkg", nil, nil)
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}

	t.Run("accepts missing credentials", func(t *testing.T) {
		t.Parallel()
		nextCalled := false
		handler := Middleware(manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !nextCalled || rec.Code != http.StatusNoContent {
			t.Fatalf("unexpected response: called=%v code=%d", nextCalled, rec.Code)
		}
	})

	t.Run("rejects invalid credentials", func(t *testing.T) {
		t.Parallel()
		handler := Middleware(manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("X-API-Key", "pk.invalid")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("code = %d, want 401", rec.Code)
		}
	})

	t.Run("stores validated key in context", func(t *testing.T) {
		t.Parallel()
		handler := Middleware(manager)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := FromContext(r.Context())
			if key == nil || key.OwnerID != "user-1" {
				t.Fatalf("unexpected context key: %+v", key)
			}
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("X-API-Key", issued.PlainKey)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("code = %d, want 204", rec.Code)
		}
	})
}
