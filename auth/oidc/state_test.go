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

func TestNewPKCE_FormatAndValidation(t *testing.T) {
	t.Parallel()
	pkce, err := oidc.NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}
	if pkce.CodeChallengeMethod != "S256" {
		t.Fatalf("method = %q, want S256", pkce.CodeChallengeMethod)
	}
	if err := oidc.ValidatePKCE(pkce); err != nil {
		t.Fatalf("valid PKCE rejected: %v", err)
	}
}

func TestValidatePKCE_Rejections(t *testing.T) {
	t.Parallel()
	cases := map[string]*oidc.PKCE{
		"empty-verifier":  {CodeChallenge: "c", CodeChallengeMethod: "S256"},
		"empty-challenge": {CodeVerifier: "v", CodeChallengeMethod: "S256"},
		"wrong-method":    {CodeVerifier: "v", CodeChallenge: "c", CodeChallengeMethod: "plain"},
	}
	for name, pkce := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := oidc.ValidatePKCE(pkce); err == nil {
				t.Fatalf("expected rejection for %s", name)
			}
		})
	}
}

func TestValidateSecretMatch_Missing(t *testing.T) {
	t.Parallel()
	if err := oidc.ValidateState("", "actual"); err == nil {
		t.Fatal("expected missing-state error")
	}
	if err := oidc.ValidateNonce("expected", ""); err == nil {
		t.Fatal("expected missing-nonce error")
	}
}
