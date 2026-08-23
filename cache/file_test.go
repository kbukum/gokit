package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistryRequiresExplicitFileRegistration(t *testing.T) {
	t.Parallel()

	reg := NewFactoryRegistry()
	if _, ok := reg.Get(ProviderFile); ok {
		t.Fatal("file backend registered without explicit RegisterFile call")
	}
	if err := RegisterFile(reg); err != nil {
		t.Fatalf("RegisterFile: %v", err)
	}
	if _, ok := reg.Get(ProviderFile); !ok {
		t.Fatal("file backend not registered")
	}
}

func TestFileStoreSetGetRoundTrip(t *testing.T) {
	t.Parallel()

	store := newTestFileStore(t, FileConfig{}, time.Unix(100, 0))
	ctx := context.Background()

	if err := store.Set(ctx, "k", []byte("value"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := store.Get(ctx, "k")
	if err != nil || !ok {
		t.Fatalf("Get: value=%q ok=%v err=%v", got, ok, err)
	}
	if !bytes.Equal(got, []byte("value")) {
		t.Fatalf("Get = %q, want %q", got, "value")
	}
}

func TestFileStoreShardedLayout(t *testing.T) {
	t.Parallel()

	store := newTestFileStore(t, FileConfig{}, time.Unix(100, 0))
	if err := store.Set(context.Background(), "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	path := store.entryPath("k")
	rel, err := filepath.Rel(store.root, path)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	shard := filepath.Dir(rel)
	if len(shard) != 2 {
		t.Fatalf("shard directory = %q, want two-character shard", shard)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("entry file not written at sharded path: %v", err)
	}
}

func TestFileStoreTTLExpiry(t *testing.T) {
	t.Parallel()

	now := time.Unix(200, 0)
	store := newTestFileStore(t, FileConfig{}, now)
	ctx := context.Background()

	if err := store.Set(ctx, "k", []byte("v"), time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if _, ok, _ := store.Get(ctx, "k"); !ok {
		t.Fatal("value expired before TTL elapsed")
	}
	store.clock = func() time.Time { return now.Add(2 * time.Second) }
	if _, ok, _ := store.Get(ctx, "k"); ok {
		t.Fatal("value not treated as expired after TTL")
	}
}

func TestFileStoreDefaultTTL(t *testing.T) {
	t.Parallel()

	now := time.Unix(300, 0)
	store := newTestFileStore(t, FileConfig{DefaultTTL: time.Second}, now)
	ctx := context.Background()

	if err := store.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	store.clock = func() time.Time { return now.Add(2 * time.Second) }
	if _, ok, _ := store.Get(ctx, "k"); ok {
		t.Fatal("ttl=0 did not adopt the store default TTL")
	}
}

func TestFileStoreMissingKeyIsMiss(t *testing.T) {
	t.Parallel()

	store := newTestFileStore(t, FileConfig{}, time.Unix(100, 0))
	if _, ok, err := store.Get(context.Background(), "absent"); ok || err != nil {
		t.Fatalf("Get(absent) = ok=%v err=%v, want miss", ok, err)
	}
}

func TestFileStoreDeleteIsIdempotent(t *testing.T) {
	t.Parallel()

	store := newTestFileStore(t, FileConfig{}, time.Unix(100, 0))
	ctx := context.Background()
	if err := store.Delete(ctx, "absent"); err != nil {
		t.Fatalf("Delete(absent): %v", err)
	}
	if err := store.Set(ctx, "k", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, _ := store.Exists(ctx, "k"); ok {
		t.Fatal("key still present after Delete")
	}
}

func TestFileStoreRejectsOversizedEntry(t *testing.T) {
	t.Parallel()

	store := newTestFileStore(t, FileConfig{MaxEntryBytes: 16}, time.Unix(100, 0))
	err := store.Set(context.Background(), "k", bytes.Repeat([]byte("x"), 64), 0)
	if err == nil {
		t.Fatal("Set accepted an entry larger than MaxEntryBytes")
	}
}

func TestFileStoreGetRejectsOversizedFileOnDisk(t *testing.T) {
	t.Parallel()

	store := newTestFileStore(t, FileConfig{MaxEntryBytes: 16}, time.Unix(100, 0))
	// Simulate a tampered/corrupt on-disk entry larger than the configured bound.
	path := store.entryPath("k")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 1024), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := store.Get(context.Background(), "k"); err == nil {
		t.Fatal("Get accepted an on-disk entry larger than MaxEntryBytes")
	}
}

func TestFileStoreDetectsKeyCollision(t *testing.T) {
	t.Parallel()

	store := newTestFileStore(t, FileConfig{}, time.Unix(100, 0))
	// Write a poisoned entry at k's path but tagged with a different stored key.
	path := store.entryPath("k")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	blob, _ := json.Marshal(fileEntry{Key: "other", Value: []byte("v")})
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := store.Get(context.Background(), "k"); err == nil {
		t.Fatal("Get did not detect a stored-key/hash collision")
	}
}

func TestFileStoreKeyPrefixIsolatesEntries(t *testing.T) {
	t.Parallel()

	a := newTestFileStoreWithRoot(t, FileConfig{KeyPrefix: "a"}, time.Unix(100, 0))
	b := newFileStore(FileConfig{Root: a.root, KeyPrefix: "b"}, func() time.Time { return time.Unix(100, 0) })
	if err := b.init(); err != nil {
		t.Fatalf("init b: %v", err)
	}
	ctx := context.Background()
	if err := a.Set(ctx, "shared", []byte("from-a"), 0); err != nil {
		t.Fatalf("Set a: %v", err)
	}
	if _, ok, _ := b.Get(ctx, "shared"); ok {
		t.Fatal("prefix b observed prefix a's entry; key namespaces are not isolated")
	}
}

func TestFileStoreCleanupExpired(t *testing.T) {
	t.Parallel()

	now := time.Unix(400, 0)
	store := newTestFileStore(t, FileConfig{}, now)
	ctx := context.Background()
	for _, k := range []string{"a", "b", "c"} {
		if err := store.Set(ctx, k, []byte("v"), time.Second); err != nil {
			t.Fatalf("Set %s: %v", k, err)
		}
	}
	store.clock = func() time.Time { return now.Add(2 * time.Second) }
	removed, err := store.CleanupExpired(ctx, 10)
	if err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if removed != 3 {
		t.Fatalf("CleanupExpired removed %d, want 3", removed)
	}
}

func newTestFileStore(t *testing.T, cfg FileConfig, now time.Time) *FileStore {
	t.Helper()
	return newTestFileStoreWithRoot(t, cfg, now)
}

func newTestFileStoreWithRoot(t *testing.T, cfg FileConfig, now time.Time) *FileStore {
	t.Helper()
	if cfg.Root == "" {
		cfg.Root = t.TempDir()
	}
	store := newFileStore(cfg, func() time.Time { return now })
	if err := store.init(); err != nil {
		t.Fatalf("init file store: %v", err)
	}
	return store
}
