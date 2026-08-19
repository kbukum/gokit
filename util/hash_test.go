package util

import (
	"bytes"
	"strings"
	"testing"
)

func TestHashHexStableAnd64Chars(t *testing.T) {
	t.Parallel()
	first := HashHex([]byte("gokit"))
	second := HashHex([]byte("gokit"))
	if first != second {
		t.Fatal("hash is not stable")
	}
	if len(first) != 64 {
		t.Fatalf("hash length = %d, want 64", len(first))
	}
	for _, c := range first {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			t.Fatalf("non lowercase-hex char %q", c)
		}
	}
}

func TestHashHexDistinctInputs(t *testing.T) {
	t.Parallel()
	if HashHex([]byte("a")) == HashHex([]byte("b")) {
		t.Fatal("distinct inputs collided")
	}
}

func TestContentHasherFinalizeDoesNotConsume(t *testing.T) {
	t.Parallel()
	h := NewContentHasher()
	h.Update([]byte("a"))
	afterA := h.FinalizeHex()
	h.Update([]byte("b"))
	afterAB := h.FinalizeHex()
	if afterA == afterAB {
		t.Fatal("finalize should not reset the hasher")
	}
}

func TestContentHasherFramingPreventsCollision(t *testing.T) {
	t.Parallel()
	left := NewContentHasher()
	left.UpdateFramed([]byte("ab"), []byte("c"))
	right := NewContentHasher()
	right.UpdateFramed([]byte("a"), []byte("bc"))
	if left.FinalizeHex() == right.FinalizeHex() {
		t.Fatal("framed fields must not alias")
	}
}

func TestContentHasherRawConcatCollides(t *testing.T) {
	t.Parallel()
	left := NewContentHasher()
	left.Update([]byte("ab")).Update([]byte("c"))
	right := NewContentHasher()
	right.Update([]byte("a")).Update([]byte("bc"))
	if left.FinalizeHex() != right.FinalizeHex() {
		t.Fatal("raw concatenation should alias without framing")
	}
}

func TestSha256KnownAnswers(t *testing.T) {
	t.Parallel()
	const abc = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := Sha256Hex([]byte("abc")); got != abc {
		t.Fatalf("Sha256Hex(abc) = %q", got)
	}
}

func TestSha256Reader(t *testing.T) {
	t.Parallel()
	payload := bytes.Repeat([]byte{0x5a}, 20000)
	got, err := Sha256Reader(strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if got != Sha256Hex(payload) {
		t.Fatal("reader digest differs from in-memory digest")
	}
}
