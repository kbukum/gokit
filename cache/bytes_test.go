package cache

import (
	"bytes"
	"context"
	"testing"
	"time"
)

// nonUTF8Payload is deliberately invalid UTF-8 and includes NUL and high bytes to
// prove the cache stores raw bytes rather than text.
var nonUTF8Payload = []byte{0x00, 0xff, 0xfe, 0x80, 0x01, 'g', 'o', 0xc0, 0xc1}

func TestMemoryStoreRoundTripsNonUTF8Bytes(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore(MemoryConfig{})
	ctx := context.Background()

	if err := store.Set(ctx, "raw", nonUTF8Payload, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := store.Get(ctx, "raw")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get: entry missing")
	}
	if !bytes.Equal(got, nonUTF8Payload) {
		t.Fatalf("Get = %v, want %v", got, nonUTF8Payload)
	}
}

func TestFileStoreRoundTripsNonUTF8Bytes(t *testing.T) {
	t.Parallel()

	store := newTestFileStore(t, FileConfig{}, time.Unix(100, 0))
	ctx := context.Background()

	if err := store.Set(ctx, "raw", nonUTF8Payload, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := store.Get(ctx, "raw")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get: entry missing")
	}
	if !bytes.Equal(got, nonUTF8Payload) {
		t.Fatalf("Get = %v, want %v", got, nonUTF8Payload)
	}
}
