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

func TestHasherConfigRedactsPepper(t *testing.T) {
	t.Parallel()

	hasher := testHasher(t)
	cfg := hasher.Config()
	if cfg.Pepper != "" {
		t.Fatalf("Config() leaked pepper: %q", cfg.Pepper)
	}
	if cfg.EntropyBytes != defaultEntropyBytes {
		t.Fatalf("EntropyBytes = %d, want %d", cfg.EntropyBytes, defaultEntropyBytes)
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

	issued, record, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "key-1", OwnerID: "user-1", Name: "primary", Prefix: "pkg", Scopes: []string{"read"}, ExpiresAt: nil})
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
	issued, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "key-1", OwnerID: "user-1", Name: "expired", Prefix: "pkg", Scopes: nil, ExpiresAt: nil})
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
	issued, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "key-1", OwnerID: "user-1", Name: "primary", Prefix: "pkg", Scopes: nil, ExpiresAt: nil})
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

func TestValidatePrefixRejectsInvalid(t *testing.T) {
	t.Parallel()
	hasher := testHasher(t)
	cases := []string{"", "ab", "bad/prefix", "bad prefix"}
	for _, prefix := range cases {
		if _, err := hasher.GenerateKey(prefix); err == nil {
			t.Fatalf("GenerateKey(%q) expected prefix rejection", prefix)
		}
	}
}

func TestIsExpiredPastGrace(t *testing.T) {
	t.Parallel()
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	if !(&Key{ExpiresAt: &past}).IsExpiredPastGrace() {
		t.Fatal("expired key without grace should be past grace")
	}
	if (&Key{ExpiresAt: &past, GraceEndsAt: &future}).IsExpiredPastGrace() {
		t.Fatal("expired key within grace should not be past grace")
	}
	if !(&Key{GraceEndsAt: &past}).IsExpiredPastGrace() {
		t.Fatal("key past grace end should be past grace")
	}
	if (&Key{ExpiresAt: &future}).IsExpiredPastGrace() {
		t.Fatal("unexpired key should not be past grace")
	}
}

func TestValidateKeyErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("malformed key", func(t *testing.T) {
		t.Parallel()
		manager := NewManager(newMemStore(), testHasher(t))
		if _, err := manager.ValidateKey(context.Background(), "no-separator"); err == nil {
			t.Fatal("expected malformed key error")
		}
	})

	t.Run("invalid prefix", func(t *testing.T) {
		t.Parallel()
		manager := NewManager(newMemStore(), testHasher(t))
		if _, err := manager.ValidateKey(context.Background(), "ab.secret"); err == nil {
			t.Fatal("expected invalid prefix error")
		}
	})

	t.Run("store list error", func(t *testing.T) {
		t.Parallel()
		store := newMemStore()
		store.listErr = errors.New("list failed")
		manager := NewManager(store, testHasher(t))
		if _, err := manager.ValidateKey(context.Background(), "pkg.secret"); err == nil {
			t.Fatal("expected store list error")
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		t.Parallel()
		manager := NewManager(newMemStore(), testHasher(t))
		if _, err := manager.ValidateKey(context.Background(), "pkg.secret"); err == nil {
			t.Fatal("expected unknown key error")
		}
	})

	t.Run("insufficient scope", func(t *testing.T) {
		t.Parallel()
		store := newMemStore()
		manager := NewManager(store, testHasher(t))
		issued, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "k1", OwnerID: "owner", Name: "name", Prefix: "pkg", Scopes: []string{"read"}, ExpiresAt: nil})
		if err != nil {
			t.Fatalf("IssueKey: %v", err)
		}
		if _, err := manager.ValidateKey(context.Background(), issued.PlainKey, "write"); err == nil {
			t.Fatal("expected insufficient scope error")
		}
	})
}

func TestIssueKeyRejectsInvalidPrefix(t *testing.T) {
	t.Parallel()
	manager := NewManager(newMemStore(), testHasher(t))
	if _, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "k1", OwnerID: "owner", Name: "name", Prefix: "ab", Scopes: nil, ExpiresAt: nil}); err == nil {
		t.Fatal("expected invalid prefix error")
	}
}

func TestValidateRejectsRevokedAndExpired(t *testing.T) {
	t.Parallel()
	if err := Validate(&Key{IsActive: false}); err == nil {
		t.Fatal("expected revoked key error")
	}
	past := time.Now().Add(-time.Hour)
	if err := Validate(&Key{IsActive: true, ExpiresAt: &past}); err == nil {
		t.Fatal("expected expired key error")
	}
}

type failingStore struct {
	*memStore
	createErr     error
	updateUsedErr error
}

func (s *failingStore) Create(ctx context.Context, key *Key) error {
	if s.createErr != nil {
		return s.createErr
	}
	return s.memStore.Create(ctx, key)
}

func (s *failingStore) UpdateLastUsed(ctx context.Context, id string, usedAt time.Time) error {
	if s.updateUsedErr != nil {
		return s.updateUsedErr
	}
	return s.memStore.UpdateLastUsed(ctx, id, usedAt)
}

func TestIssueKeyPropagatesCreateError(t *testing.T) {
	t.Parallel()
	store := &failingStore{memStore: newMemStore(), createErr: errors.New("create failed")}
	manager := NewManager(store, testHasher(t))
	if _, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "k1", OwnerID: "owner", Name: "name", Prefix: "pkg", Scopes: nil, ExpiresAt: nil}); err == nil {
		t.Fatal("expected create error to propagate")
	}
}

func TestValidateKeyPropagatesUpdateLastUsedError(t *testing.T) {
	t.Parallel()
	store := &failingStore{memStore: newMemStore()}
	manager := NewManager(store, testHasher(t))
	issued, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "k1", OwnerID: "owner", Name: "name", Prefix: "pkg", Scopes: nil, ExpiresAt: nil})
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}
	store.updateUsedErr = errors.New("update failed")
	if _, err := manager.ValidateKey(context.Background(), issued.PlainKey); err == nil {
		t.Fatal("expected UpdateLastUsed error to propagate")
	}
}

func FuzzSplitKey(f *testing.F) {
	f.Add("pk.secret")
	f.Add("malformed")
	f.Fuzz(func(t *testing.T, plain string) {
		_, _, _ = SplitKey(plain)
	})
}

func FuzzDigestCompare(f *testing.F) {
	hasher, err := NewHasher(HashingConfig{Pepper: "pppppppppppppppppppppppppppppppp"})
	if err != nil {
		f.Fatalf("NewHasher: %v", err)
	}
	f.Add("pk.secret")
	f.Fuzz(func(t *testing.T, plain string) {
		digest := hasher.Digest(plain)
		_ = hasher.Compare(plain, digest)
	})
}
