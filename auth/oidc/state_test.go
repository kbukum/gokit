package oidc_test

import (
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
)

func TestGenerateState_FormatAndUniqueness(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		s, err := oidc.GenerateState()
		if err != nil {
			t.Fatalf("GenerateState: %v", err)
		}
		if len(s) != 64 {
			t.Errorf("length: got %d want 64", len(s))
		}
		if seen[s] {
			t.Fatalf("duplicate state: %s", s)
		}
		seen[s] = true
	}
}

func TestGenerateNonce_FormatAndUniqueness(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		n, err := oidc.GenerateNonce()
		if err != nil {
			t.Fatalf("GenerateNonce: %v", err)
		}
		if len(n) != 32 {
			t.Errorf("length: got %d want 32", len(n))
		}
		if seen[n] {
			t.Fatalf("duplicate nonce: %s", n)
		}
		seen[n] = true
	}
}
