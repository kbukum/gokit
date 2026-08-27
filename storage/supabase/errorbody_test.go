package supabase

import (
	"bytes"
	"testing"
)

func TestReadErrorBody_CapsAtLimit(t *testing.T) {
	t.Parallel()

	oversized := bytes.NewReader(bytes.Repeat([]byte("e"), maxErrorBodyBytes*2))
	got := readErrorBody(oversized)
	if len(got) != maxErrorBodyBytes {
		t.Fatalf("read %d bytes, want cap of %d", len(got), maxErrorBodyBytes)
	}
}

func TestReadErrorBody_SmallBodyIntact(t *testing.T) {
	t.Parallel()

	want := []byte("bucket not found")
	got := readErrorBody(bytes.NewReader(want))
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
