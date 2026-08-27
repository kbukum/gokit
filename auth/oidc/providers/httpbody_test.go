package providers

import (
	"bytes"
	"testing"
)

func TestReadResponseBody_CapsAtLimit(t *testing.T) {
	t.Parallel()

	// A body larger than the cap must be bounded, so a hostile or misbehaving
	// identity provider cannot exhaust memory.
	oversized := bytes.NewReader(bytes.Repeat([]byte("a"), maxResponseBytes*2))

	got, err := readResponseBody(oversized)
	if err != nil {
		t.Fatalf("readResponseBody: %v", err)
	}
	if len(got) != maxResponseBytes {
		t.Fatalf("read %d bytes, want cap of %d", len(got), maxResponseBytes)
	}
}

func TestReadResponseBody_SmallBodyIntact(t *testing.T) {
	t.Parallel()

	want := []byte(`{"access_token":"abc"}`)
	got, err := readResponseBody(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("readResponseBody: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
