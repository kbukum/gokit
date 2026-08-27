package providers

import (
	"bytes"
	"errors"
	"testing"
)

func TestReadResponseBody_RejectsOversize(t *testing.T) {
	t.Parallel()

	// A body larger than the cap must be rejected outright, not truncated to a
	// (possibly still-decodable) prefix, so a hostile or misbehaving identity
	// provider cannot slip a valid JSON prefix past the size guard or exhaust memory.
	oversized := bytes.NewReader(bytes.Repeat([]byte("a"), maxResponseBytes*2))

	if _, err := readResponseBody(oversized); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("readResponseBody error = %v, want ErrResponseTooLarge", err)
	}
}

func TestReadResponseBody_AtLimitIntact(t *testing.T) {
	t.Parallel()

	// A body exactly at the cap is read in full.
	want := bytes.Repeat([]byte("a"), maxResponseBytes)
	got, err := readResponseBody(bytes.NewReader(want))
	if err != nil {
		t.Fatalf("readResponseBody: %v", err)
	}
	if len(got) != maxResponseBytes {
		t.Fatalf("read %d bytes, want %d", len(got), maxResponseBytes)
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
