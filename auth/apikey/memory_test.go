package apikey

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

func TestMemoryStoreIndexOwnershipCapacityAndDelete(t *testing.T) {
	store, err := NewMemoryStore(1)
	if err != nil {
		t.Fatal(err)
	}
	clock := util.NewFakeClock(time.Time{})
	manager := NewManager(store, testHasher(t), WithClock(clock))
	expiry := clock.Now().Add(time.Hour)
	issued, key, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "one", OwnerID: "owner", Prefix: "key", Kind: auth.User, RestrictionMode: auth.Restricted, Resources: []string{"one"}, Scopes: []string{"read"}, ExpiresAt: &expiry})
	if err != nil {
		t.Fatal(err)
	}
	key.Resources[0] = "changed"
	key.Scopes[0] = "write"
	*key.ExpiresAt = clock.Now()
	fromStore, err := store.GetByDigest(context.Background(), issued.KeyDigest)
	if err != nil || fromStore.Resources[0] != "one" || fromStore.Scopes[0] != "read" || !fromStore.ExpiresAt.Equal(expiry) {
		t.Fatal("write alias", err)
	}
	fromStore.Resources[0] = "changed"
	*fromStore.ExpiresAt = clock.Now()
	valid, err := manager.ValidateKey(context.Background(), issued.PlainKey, "read")
	if err != nil || valid.Resources[0] != "one" {
		t.Fatal("read alias", err)
	}
	if _, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "two", OwnerID: "owner", Prefix: "key"}); err == nil {
		t.Fatal("capacity unbounded")
	}
	if err := store.Delete(context.Background(), "one"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetByDigest(context.Background(), issued.KeyDigest); apperrors.Normalize(err).Code != apperrors.ErrCodeNotFound {
		t.Fatal("stale digest index", err)
	}
	if _, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "two", OwnerID: "owner", Prefix: "key"}); err != nil {
		t.Fatal("capacity not released", err)
	}
}

func TestMemoryStoreMutationCancellationAndIsolation(t *testing.T) {
	store, err := NewMemoryStore(0)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(store, testHasher(t))
	issued, key, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: "one", OwnerID: "owner", Prefix: "key"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), key); err == nil {
		t.Fatal("duplicate ID")
	}
	cloned := key.Clone()
	cloned.ID = "other"
	if err := store.Create(context.Background(), cloned); err == nil {
		t.Fatal("duplicate digest")
	}
	now := time.Now().UTC()
	if err := store.UpdateLastUsed(context.Background(), key.ID, now); err != nil {
		t.Fatal(err)
	}
	if err := store.SetRotation(context.Background(), key.ID, now.Add(time.Minute), "replacement"); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByID(context.Background(), key.ID)
	if err != nil || got.LastUsedAt == nil || !got.LastUsedAt.Equal(now) || got.RotatedByID != "replacement" {
		t.Fatal(err)
	}
	if err := store.SetActive(context.Background(), key.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ValidateKey(context.Background(), issued.PlainKey); err == nil {
		t.Fatal("disabled key accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.SetActive(ctx, key.ID, true); err == nil {
		t.Fatal("canceled mutation")
	}
	if _, err := store.GetByID(ctx, key.ID); err == nil {
		t.Fatal("canceled read")
	}
	if _, err := store.GetByDigest(ctx, issued.KeyDigest); err == nil {
		t.Fatal("canceled digest read")
	}
	if err := store.Create(ctx, cloned); err == nil {
		t.Fatal("canceled create")
	}
	if err := store.Delete(ctx, key.ID); err == nil {
		t.Fatal("canceled delete")
	}
	if err := store.UpdateLastUsed(ctx, key.ID, now); err == nil {
		t.Fatal("canceled audit")
	}
	if err := store.SetRotation(ctx, key.ID, now, "replacement"); err == nil {
		t.Fatal("canceled rotation")
	}
	if err := store.SetActive(context.Background(), key.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ValidateKey(context.Background(), issued.PlainKey); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func() error{
		func() error { return store.Delete(context.Background(), "absent") },
		func() error { return store.SetActive(context.Background(), "absent", false) },
		func() error { return store.SetRotation(context.Background(), "absent", now, "new") },
		func() error { return store.UpdateLastUsed(context.Background(), "absent", now) },
	} {
		if err := operation(); apperrors.Normalize(err).Code != apperrors.ErrCodeNotFound {
			t.Fatal(err)
		}
	}
}

func TestMemoryStoreBoundsAndConcurrentAdmission(t *testing.T) {
	for _, capacity := range []int{-1, 4097} {
		if _, err := NewMemoryStore(capacity); err == nil {
			t.Fatal("invalid capacity")
		}
	}
	store, err := NewMemoryStore(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), nil); err == nil {
		t.Fatal("nil record")
	}
	manager := NewManager(store, testHasher(t))
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range []string{"one", "two"} {
		wg.Go(func() {
			_, _, err := manager.IssueKey(context.Background(), IssueRequest{KeyID: id, OwnerID: "owner", Prefix: "key"})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("admission not atomic", success)
	}
}
