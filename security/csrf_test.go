package security

import (
	"bytes"
	"strings"
	"testing"
)

func TestSignedCSRF(t *testing.T) {
	t.Parallel()
	s, err := NewSignedCSRF(bytes.Repeat([]byte{1}, 32), bytes.NewReader(bytes.Repeat([]byte{2}, 128)))
	if err != nil {
		t.Fatal(err)
	}
	token, err := s.Issue("generation")
	if err != nil || len(token) > 256 {
		t.Fatal(err)
	}
	if err := s.Verify("generation", token); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"other", token}, {"generation", token + "x"}, {"generation", ""}, {"generation", strings.Repeat("x", 257)}} {
		if s.Verify(pair[0], pair[1]) == nil {
			t.Fatal("accepted invalid CSRF")
		}
	}
	if _, err := NewSignedCSRF([]byte("short"), bytes.NewReader(nil)); err == nil {
		t.Fatal("weak key")
	}
	if _, err := NewSignedCSRF(bytes.Repeat([]byte{1}, 32), nil); err == nil {
		t.Fatal("missing reader")
	}
	s, _ = NewSignedCSRF(bytes.Repeat([]byte{1}, 32), bytes.NewReader(nil))
	if _, err := s.Issue("generation"); err == nil {
		t.Fatal("entropy failure")
	}
}

func FuzzSignedCSRF(f *testing.F) {
	f.Add("generation", "invalid")
	s, _ := NewSignedCSRF(bytes.Repeat([]byte{1}, 32), bytes.NewReader(nil))
	f.Fuzz(func(t *testing.T, generation, token string) { _ = s.Verify(generation, token) })
}
