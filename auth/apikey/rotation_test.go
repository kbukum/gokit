package apikey

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRotateKeyRequiresNewKeyID(t *testing.T) {
	t.Parallel()
	manager := NewManager(newMemStore(), testHasher(t))
	if _, err := manager.RotateKey(context.Background(), "old", RotationConfig{}); err == nil {
		t.Fatal("expected NewKeyID required error")
	}
}

func TestRotateKeyFailsWhenOldKeyMissing(t *testing.T) {
	t.Parallel()
	manager := NewManager(newMemStore(), testHasher(t))
	_, err := manager.RotateKey(context.Background(), "missing", RotationConfig{NewKeyID: "new"})
	if err == nil {
		t.Fatal("expected error when old key is missing")
	}
}

func TestRotateKeyRejectsRevokedOldKey(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	manager := NewManager(store, testHasher(t))
	_, record, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "old", OwnerID: "owner", Name: "name", Prefix: "pkg", Scopes: []string{"read"}, ExpiresAt: nil})
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}
	if setErr := store.SetActive(context.Background(), record.ID, false); setErr != nil {
		t.Fatalf("SetActive: %v", setErr)
	}
	if _, rotErr := manager.RotateKey(context.Background(), record.ID, RotationConfig{NewKeyID: "new"}); rotErr == nil {
		t.Fatal("expected revoked old key rejection")
	}
}

func TestRotateKeyInheritsOldMetadata(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	manager := NewManager(store, testHasher(t))
	expires := time.Now().Add(time.Hour)
	_, record, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "old", OwnerID: "owner", Name: "primary", Prefix: "pkg", Scopes: []string{"read", "write"}, ExpiresAt: &expires})
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}

	result, err := manager.RotateKey(context.Background(), record.ID, RotationConfig{NewKeyID: "new"})
	if err != nil {
		t.Fatalf("RotateKey: %v", err)
	}
	if result.Record.OwnerID != "owner" || result.Record.Name != "primary" || result.Record.KeyPrefix != "pkg" {
		t.Fatalf("rotated record did not inherit metadata: %+v", result.Record)
	}
	if len(result.Record.Scopes) != 2 {
		t.Fatalf("scopes = %v, want inherited 2", result.Record.Scopes)
	}
	if !result.GraceEndsAt.After(time.Now()) {
		t.Fatal("grace window should be in the future")
	}
}

func TestRotateKeyPropagatesIssueError(t *testing.T) {
	t.Parallel()
	store := newMemStore()
	manager := NewManager(store, testHasher(t))
	_, record, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "old", OwnerID: "owner", Name: "name", Prefix: "pkg", Scopes: nil, ExpiresAt: nil})
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}
	// A prefix shorter than 3 chars forces GenerateKey (via IssueKey) to fail.
	_, rotErr := manager.RotateKey(context.Background(), record.ID, RotationConfig{NewKeyID: "new", Prefix: "ab"})
	if rotErr == nil {
		t.Fatal("expected issue error from invalid prefix")
	}
}

func TestRotateKeyPropagatesSetRotationError(t *testing.T) {
	t.Parallel()
	store := &rotationErrStore{memStore: newMemStore()}
	manager := NewManager(store, testHasher(t))
	_, record, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "old", OwnerID: "owner", Name: "name", Prefix: "pkg", Scopes: nil, ExpiresAt: nil})
	if err != nil {
		t.Fatalf("IssueKey: %v", err)
	}
	store.setRotationErr = errors.New("set rotation failed")
	if _, rotErr := manager.RotateKey(context.Background(), record.ID, RotationConfig{NewKeyID: "new"}); rotErr == nil {
		t.Fatal("expected SetRotation error to propagate")
	}
}

type rotationErrStore struct {
	*memStore
	setRotationErr error
}

func (s *rotationErrStore) SetRotation(ctx context.Context, id string, graceEndsAt time.Time, rotatedByID string) error {
	if s.setRotationErr != nil {
		return s.setRotationErr
	}
	return s.memStore.SetRotation(ctx, id, graceEndsAt, rotatedByID)
}
