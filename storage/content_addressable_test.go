package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"hash/fnv"
	"io"
	"strings"
	"testing"
)

func TestContentAddressableStorage_Store_NewContent(t *testing.T) {
	t.Parallel()

	ms := newMockStorage()
	cas := NewContentAddressableStorage(ms)

	content := []byte("hello world")
	h, isNew, err := cas.Store(context.Background(), bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true for first store")
	}

	// Verify hash matches sha256.
	expected := sha256Hex(content)
	if h != expected {
		t.Errorf("hash = %q, want %q", h, expected)
	}

	// Verify content is stored at the correct key.
	key := "sha256/" + expected
	if _, ok := ms.data[key]; !ok {
		t.Errorf("expected content at key %q", key)
	}
}

func TestContentAddressableStorage_Store_Deduplication(t *testing.T) {
	t.Parallel()

	ms := newMockStorage()
	cas := NewContentAddressableStorage(ms)
	ctx := context.Background()

	content := []byte("duplicate me")

	h1, isNew1, err := cas.Store(ctx, bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("Store 1: %v", err)
	}
	if !isNew1 {
		t.Error("expected isNew=true on first store")
	}

	h2, isNew2, err := cas.Store(ctx, bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("Store 2: %v", err)
	}
	if isNew2 {
		t.Error("expected isNew=false on duplicate store")
	}
	if h1 != h2 {
		t.Errorf("hashes differ: %q vs %q", h1, h2)
	}
}

func TestContentAddressableStorage_Get(t *testing.T) {
	t.Parallel()

	ms := newMockStorage()
	cas := NewContentAddressableStorage(ms)
	ctx := context.Background()

	content := []byte("retrieve me")
	h, _, err := cas.Store(ctx, bytes.NewReader(content), "application/octet-stream")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	rc, err := cas.Get(ctx, h)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %q, want %q", got, content)
	}
}

func TestContentAddressableStorage_Get_NotFound(t *testing.T) {
	t.Parallel()

	ms := newMockStorage()
	cas := NewContentAddressableStorage(ms)

	_, err := cas.Get(context.Background(), "deadbeef")
	if err == nil {
		t.Fatal("expected error for missing hash")
	}
}

func TestContentAddressableStorage_Exists(t *testing.T) {
	t.Parallel()

	ms := newMockStorage()
	cas := NewContentAddressableStorage(ms)
	ctx := context.Background()

	exists, err := cas.Exists(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Error("expected false for nonexistent hash")
	}

	h, _, err := cas.Store(ctx, bytes.NewReader([]byte("data")), "")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	exists, err = cas.Exists(ctx, h)
	if err != nil {
		t.Fatalf("Exists after store: %v", err)
	}
	if !exists {
		t.Error("expected true after store")
	}
}

func TestContentAddressableStorage_Delete(t *testing.T) {
	t.Parallel()

	ms := newMockStorage()
	cas := NewContentAddressableStorage(ms)
	ctx := context.Background()

	h, _, err := cas.Store(ctx, bytes.NewReader([]byte("deletable")), "")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	if err := cas.Delete(ctx, h); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	exists, _ := cas.Exists(ctx, h)
	if exists {
		t.Error("expected content deleted")
	}
}

func TestContentAddressableStorage_URL(t *testing.T) {
	t.Parallel()

	ms := newMockStorage()
	cas := NewContentAddressableStorage(ms)
	ctx := context.Background()

	h, _, err := cas.Store(ctx, bytes.NewReader([]byte("url-test")), "")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	u, err := cas.URL(ctx, h)
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	if !strings.Contains(u, h) {
		t.Errorf("URL %q should contain hash %q", u, h)
	}
}

func TestContentAddressableStorage_CustomHasher(t *testing.T) {
	t.Parallel()

	ms := newMockStorage()
	cas := NewContentAddressableStorage(ms,
		WithHasher(fnv.New128),
		WithPrefix("fnv128/"),
	)
	ctx := context.Background()

	h, isNew, err := cas.Store(ctx, bytes.NewReader([]byte("custom hash")), "")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true")
	}
	if h == "" {
		t.Error("expected non-empty hash")
	}

	// Verify stored under custom prefix.
	key := "fnv128/" + h
	if _, ok := ms.data[key]; !ok {
		t.Errorf("expected content at key %q", key)
	}
}

func TestContentAddressableStorage_EmptyContent(t *testing.T) {
	t.Parallel()

	ms := newMockStorage()
	cas := NewContentAddressableStorage(ms)
	ctx := context.Background()

	h, isNew, err := cas.Store(ctx, bytes.NewReader([]byte{}), "")
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !isNew {
		t.Error("expected isNew=true")
	}

	expected := sha256Hex([]byte{})
	if h != expected {
		t.Errorf("empty hash = %q, want %q", h, expected)
	}
}

// sha256Hex computes the hex-encoded SHA-256 hash of data.
func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
