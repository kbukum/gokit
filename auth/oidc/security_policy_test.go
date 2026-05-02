package oidc_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/oidc"
)

func TestValidateStateAndNonce(t *testing.T) {
	t.Parallel()

	if err := oidc.ValidateState("expected", "expected"); err != nil {
		t.Fatalf("ValidateState success: %v", err)
	}
	if err := oidc.ValidateNonce("nonce", "nonce"); err != nil {
		t.Fatalf("ValidateNonce success: %v", err)
	}
	if err := oidc.ValidateState("expected", "actual"); err == nil {
		t.Fatal("expected state mismatch to fail")
	}
	if err := oidc.ValidateNonce("expected", "actual"); err == nil {
		t.Fatal("expected nonce mismatch to fail")
	}
}

func TestValidatePKCE(t *testing.T) {
	t.Parallel()

	if err := oidc.ValidatePKCE(nil); err == nil {
		t.Fatal("expected nil PKCE to fail")
	}
	pkce, err := oidc.NewPKCE()
	if err != nil {
		t.Fatalf("NewPKCE: %v", err)
	}
	if err := oidc.ValidatePKCE(pkce); err != nil {
		t.Fatalf("expected valid PKCE, got %v", err)
	}
}

func TestVerifier_VerifyExpectedRejectsNonceMismatch(t *testing.T) {
	t.Parallel()

	kit := newRSATestKit(t)
	v, err := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	tok := kit.signRS256(t, kit.kid, map[string]any{
		"iss":   kit.issuer(),
		"sub":   "user-1",
		"aud":   "cid",
		"exp":   float64(time.Now().Add(time.Hour).Unix()),
		"iat":   float64(time.Now().Unix()),
		"nbf":   float64(time.Now().Unix()),
		"nonce": "actual-nonce",
	})

	if _, err := v.VerifyExpected(context.Background(), tok, oidc.VerifyExpectations{Nonce: "expected-nonce"}); err == nil {
		t.Fatal("expected nonce mismatch to fail")
	}
}

func TestConfig_Validate_PublicClientRequiresNoSecret(t *testing.T) {
	t.Parallel()

	cfg := oidc.Config{
		Enabled:      true,
		Issuer:       "https://issuer.example.com",
		ClientID:     "client-id",
		ClientSecret: "secret",
		PublicClient: true,
		RedirectURL:  "https://app.example.com/callback",
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected public client with secret to fail")
	}
}

func TestVerifier_Verify_EdDSA(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	var srvURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   srvURL,
			"jwks_uri": srvURL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "OKP", "kid": "eddsa-1", "alg": "EdDSA", "use": "sig",
				"crv": "Ed25519",
				"x":   base64.RawURLEncoding.EncodeToString(pub),
			}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	v, err := oidc.NewVerifier(context.Background(), srvURL, oidc.VerifierConfig{
		ClientID:             "cid",
		SupportedSigningAlgs: []string{"EdDSA"},
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	header, _ := json.Marshal(map[string]any{"alg": "EdDSA", "typ": "JWT", "kid": "eddsa-1"})
	payload, _ := json.Marshal(map[string]any{
		"iss": srvURL, "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
		"iat": float64(time.Now().Unix()),
		"nbf": float64(time.Now().Unix()),
	})
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	signature := ed25519.Sign(priv, []byte(signingInput))
	token := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)

	if _, err := v.Verify(context.Background(), token); err != nil {
		t.Fatalf("EdDSA verify: %v", err)
	}
}

func TestConfig_Validate_RejectsWildcardRedirectURI(t *testing.T) {
	t.Parallel()

	cfg := oidc.Config{
		Enabled:     true,
		Issuer:      "https://issuer.example.com",
		ClientID:    "client-id",
		RedirectURL: "https://*.example.com/callback",
	}
	cfg.ApplyDefaults()
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "exact absolute URI") {
		t.Fatalf("expected wildcard redirect rejection, got %v", err)
	}
}
