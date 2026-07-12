package oidc_test

import (
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
)

func TestApplyAuthURLOptions(t *testing.T) {
	t.Parallel()
	pkce, _ := oidc.NewPKCE()
	o := oidc.ApplyAuthURLOptions([]oidc.AuthURLOption{
		oidc.WithRedirectURI("https://cb"),
		oidc.WithScopes("openid", "email"),
		oidc.WithNonce("n123"),
		oidc.WithPKCE(pkce),
		oidc.WithExtraParam("prompt", "consent"),
		oidc.WithExtraParam("login_hint", "user@example.com"),
	})
	if o.RedirectURI != "https://cb" {
		t.Errorf("RedirectURI: %q", o.RedirectURI)
	}
	if !equalStrings(o.Scopes, []string{"openid", "email"}) {
		t.Errorf("Scopes: %v", o.Scopes)
	}
	if o.Nonce != "n123" {
		t.Errorf("Nonce: %q", o.Nonce)
	}
	if o.PKCE != pkce {
		t.Errorf("PKCE pointer not set")
	}
	if o.ExtraParams["prompt"] != "consent" || o.ExtraParams["login_hint"] != "user@example.com" {
		t.Errorf("ExtraParams: %v", o.ExtraParams)
	}
}

func TestApplyAuthURLOptions_Empty(t *testing.T) {
	t.Parallel()
	o := oidc.ApplyAuthURLOptions(nil)
	if o.RedirectURI != "" || o.Nonce != "" || o.PKCE != nil || len(o.Scopes) != 0 || o.ExtraParams != nil {
		t.Errorf("zero-value expected, got %+v", o)
	}
}

func TestApplyExchangeOptions(t *testing.T) {
	t.Parallel()
	o := oidc.ApplyExchangeOptions([]oidc.ExchangeOption{
		oidc.WithExchangeRedirectURI("https://cb"),
		oidc.WithCodeVerifier("verifier-x"),
	})
	if o.RedirectURI != "https://cb" {
		t.Errorf("RedirectURI: %q", o.RedirectURI)
	}
	if o.CodeVerifier != "verifier-x" {
		t.Errorf("CodeVerifier: %q", o.CodeVerifier)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// idtoken.go: ParseIDTokenClaims
// ─────────────────────────────────────────────────────────────────────────────

// makeUnsignedJWT builds a "alg:none" JWT for parse-only tests.
