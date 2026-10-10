package apikey

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestMemoryStoreCapacityIsAtomicAndReleasedByDelete(t *testing.T) {
	t.Parallel()
	for _, capacity := range []int{-1, 4097} {
		if _, err := NewMemoryStore(capacity); err == nil {
			t.Fatalf("capacity %d accepted", capacity)
		}
	}
	store, err := NewMemoryStore(1)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestManager(t, store, nil)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, id := range []string{"one", "two"} {
		wg.Go(func() {
			_, _, errs[i] = m.IssueKey(context.Background(), IssueRequest{KeyID: id, OwnerID: "owner", Prefix: "key"})
		})
	}
	wg.Wait()
	winner := "one"
	if errs[0] != nil {
		winner = "two"
	}
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("admission not atomic: %v", errs)
	}
	if _, err := m.RotateKey(context.Background(), winner, RotateRequest{NewKeyID: "three"}); reason(err) != "API_KEY_CAPACITY" {
		t.Fatalf("rotation beyond capacity: %v", err)
	}
	if err := store.Delete(context.Background(), winner); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.IssueKey(context.Background(), IssueRequest{KeyID: "four", OwnerID: "owner", Prefix: "key"}); err != nil {
		t.Fatalf("capacity not released: %v", err)
	}
}

func TestMemoryStoreRejectsInvalidMetadata(t *testing.T) {
	t.Parallel()
	store := newMemory(t)
	valid := func() *Key {
		return &Key{ID: "k", OwnerID: "o", KeyPrefix: "key", KeyDigest: strings.Repeat("ab", 32), Kind: "user", RestrictionMode: "unrestricted"}
	}
	cases := map[string]func(*Key){
		"nil":            nil,
		"empty id":       func(k *Key) { k.ID = "" },
		"long owner":     func(k *Key) { k.OwnerID = strings.Repeat("o", 513) },
		"bad prefix":     func(k *Key) { k.KeyPrefix = "a b" },
		"non-hex digest": func(k *Key) { k.KeyDigest = strings.Repeat("zz", 32) },
		"upper digest":   func(k *Key) { k.KeyDigest = strings.Repeat("AB", 32) },
		"bad kind":       func(k *Key) { k.Kind = "robot" },
		"long scope":     func(k *Key) { k.RestrictionMode, k.Scopes = "restricted", []string{strings.Repeat("s", 513)} },
		"oversized total": func(k *Key) {
			k.RestrictionMode, k.Scopes = "restricted", []string{strings.Repeat("s", 500), strings.Repeat("t", 500)}
			for range 40 {
				k.Resources = append(k.Resources, strings.Repeat("r", 500))
			}
		},
	}
	for name, mutate := range cases {
		var key *Key
		if mutate != nil {
			key = valid()
			mutate(key)
		}
		if err := store.Create(context.Background(), key); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("%s: %v", name, err)
		}
		if err := store.Rotate(context.Background(), Rotation{OldID: "k", Replacement: key}); err == nil {
			t.Fatalf("%s replacement accepted", name)
		}
	}
}

func TestMemoryStoreRotationGuards(t *testing.T) {
	t.Parallel()
	store := newMemory(t)
	key := func(id, digest string) *Key {
		return &Key{ID: id, OwnerID: "o", KeyPrefix: "key", KeyDigest: strings.Repeat(digest, 32), Kind: "user", RestrictionMode: "unrestricted"}
	}
	old := key("old", "ab")
	old.ExpiresAt = at(epoch.Add(time.Minute))
	if err := store.Create(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	for name, r := range map[string]Rotation{
		"replacement reuses the old id": {OldID: "old", Replacement: key("old", "cd"), At: epoch, GraceEndsAt: epoch},
		"replacement already rotated": {OldID: "old", Replacement: func() *Key {
			k := key("new", "cd")
			k.RotatedByID = "other"
			return k
		}(), At: epoch, GraceEndsAt: epoch},
		"grace beyond old expiry": {OldID: "old", Replacement: key("new", "cd"), At: epoch, GraceEndsAt: epoch.Add(2 * time.Minute)},
	} {
		if err := store.Rotate(context.Background(), r); apperrors.Normalize(err).Code != apperrors.ErrCodeInvalidInput {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := store.GetByID(context.Background(), "new"); err == nil {
		t.Fatal("a rejected rotation stored its replacement")
	}
}
