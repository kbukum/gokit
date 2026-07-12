package oidc_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kbukum/gokit/auth/oidc"
)

func makeUnsignedJWT(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	body, _ := json.Marshal(claims)
	payload := base64.RawURLEncoding.EncodeToString(body)
	sig := base64.RawURLEncoding.EncodeToString([]byte("nope"))
	return header + "." + payload + "." + sig
}

func TestParseIDTokenClaims_Standard(t *testing.T) {
	t.Parallel()
	tok := makeUnsignedJWT(map[string]any{
		"sub":            "s1",
		"email":          "u@example.com",
		"email_verified": true,
		"name":           "User One",
		"given_name":     "User",
		"family_name":    "One",
		"picture":        "https://p",
		"locale":         "en-US",
	})
	got, err := oidc.ParseIDTokenClaims(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Subject != "s1" || got.Email != "u@example.com" || !got.EmailVerified {
		t.Errorf("basic claims wrong: %+v", got)
	}
	if got.Name != "User One" || got.GivenName != "User" || got.FamilyName != "One" {
		t.Errorf("name claims wrong: %+v", got)
	}
	if got.Picture != "https://p" || got.Locale != "en-US" {
		t.Errorf("misc claims wrong: %+v", got)
	}
}

// Some providers return email_verified as a string ("true"/"false").

func TestParseIDTokenClaims_EmailVerifiedString(t *testing.T) {
	t.Parallel()
	tok := makeUnsignedJWT(map[string]any{"sub": "s", "email_verified": "true"})
	got, err := oidc.ParseIDTokenClaims(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !got.EmailVerified {
		t.Errorf("email_verified=\"true\" should parse to true")
	}

	tok2 := makeUnsignedJWT(map[string]any{"sub": "s", "email_verified": "false"})
	got2, _ := oidc.ParseIDTokenClaims(tok2)
	if got2.EmailVerified {
		t.Errorf("email_verified=\"false\" should parse to false")
	}
}

func TestParseIDTokenClaims_MalformedFormat(t *testing.T) {
	t.Parallel()
	_, err := oidc.ParseIDTokenClaims("not.a.jwt.too.many")
	if err == nil || !strings.Contains(err.Error(), "expected 3 parts") {
		t.Errorf("got %v want expected-3-parts error", err)
	}
}

func TestParseIDTokenClaims_BadBase64(t *testing.T) {
	t.Parallel()
	_, err := oidc.ParseIDTokenClaims("a.!!!.c")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("got %v want decode error", err)
	}
}

func TestParseIDTokenClaims_BadJSON(t *testing.T) {
	t.Parallel()
	bad := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	_, err := oidc.ParseIDTokenClaims("hdr." + bad + ".sig")
	if err == nil || !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("got %v want unmarshal error", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Verifier end-to-end with real RSA + JWKS server
// ─────────────────────────────────────────────────────────────────────────────

// rsaTestKit holds a generated RSA key + a httptest server publishing a JWKS
// and OIDC discovery document signed with that key.
