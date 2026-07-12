package oidc_test

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/oidc"
)

func TestNewPKCE_S256ChallengeMatchesVerifier(t *testing.T) {
	t.Parallel()
	p, err := oidc.NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}
	if p.CodeChallengeMethod != "S256" {
		t.Errorf("method: got %q want S256", p.CodeChallengeMethod)
	}
	// Verifier should be 43 base64url chars from 32 random bytes.
	if len(p.CodeVerifier) != 43 {
		t.Errorf("verifier length: got %d want 43", len(p.CodeVerifier))
	}
	// Challenge should be SHA-256(verifier) base64url-encoded.
	h := sha256.Sum256([]byte(p.CodeVerifier))
	want := base64.RawURLEncoding.EncodeToString(h[:])
	if p.CodeChallenge != want {
		t.Errorf("challenge mismatch")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// pkce_store.go
// ─────────────────────────────────────────────────────────────────────────────

func TestPKCEStore_SaveAndPop(t *testing.T) {
	t.Parallel()
	s := oidc.NewPKCEStore(time.Minute)
	s.Save("st", "v")
	if got := s.Pop("st"); got != "v" {
		t.Errorf("Pop: got %q want v", got)
	}
	// Pop is single-use.
	if got := s.Pop("st"); got != "" {
		t.Errorf("second Pop should be empty, got %q", got)
	}
}

func TestPKCEStore_Pop_MissingKey(t *testing.T) {
	t.Parallel()
	s := oidc.NewPKCEStore(time.Minute)
	if got := s.Pop("nope"); got != "" {
		t.Errorf("missing key: got %q want empty", got)
	}
}

func TestPKCEStore_Pop_Expired(t *testing.T) {
	t.Parallel()
	s := oidc.NewPKCEStore(1 * time.Nanosecond)
	s.Save("st", "v")
	time.Sleep(5 * time.Millisecond)
	if got := s.Pop("st"); got != "" {
		t.Errorf("expired entry should return empty, got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// provider.go: AuthURLOption / ExchangeOption builders
// ─────────────────────────────────────────────────────────────────────────────
