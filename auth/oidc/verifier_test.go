package oidc_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kbukum/gokit/auth/oidc"
)

type rsaTestKit struct {
	priv   *rsa.PrivateKey
	kid    string
	server *httptest.Server
	hits   atomic.Int32 // counts JWKS fetches
}

func newRSATestKit(t *testing.T) *rsaTestKit {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	kit := &rsaTestKit{priv: priv, kid: "test-kid"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                kit.issuer(),
			"authorization_endpoint":                kit.server.URL + "/authorize",
			"token_endpoint":                        kit.server.URL + "/token",
			"userinfo_endpoint":                     kit.server.URL + "/userinfo",
			"jwks_uri":                              kit.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		kit.hits.Add(1)
		nB := base64.RawURLEncoding.EncodeToString(priv.N.Bytes())
		eBig := big.NewInt(int64(priv.E))
		eB := base64.RawURLEncoding.EncodeToString(eBig.Bytes())
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA", "kid": kit.kid, "alg": "RS256", "use": "sig",
				"n": nB, "e": eB,
			}},
		})
	})
	srv := httptest.NewServer(mux)
	kit.server = srv
	t.Cleanup(srv.Close)
	return kit
}

func (k *rsaTestKit) issuer() string {
	if k.server == nil {
		return ""
	}
	return k.server.URL
}

// signRS256 produces a real RS256 JWT for the given claims and key id.

func (k *rsaTestKit) signRS256(t *testing.T, kid string, claims map[string]any) string {
	t.Helper()
	hdr := map[string]any{"alg": "RS256", "typ": "JWT", "kid": kid}
	hb, _ := json.Marshal(hdr)
	pb, _ := json.Marshal(claims)
	headerB := base64.RawURLEncoding.EncodeToString(hb)
	payloadB := base64.RawURLEncoding.EncodeToString(pb)
	signingInput := headerB + "." + payloadB
	h := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, k.priv, crypto.SHA256, h[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestNewVerifier_RequiresClientID(t *testing.T) {
	t.Parallel()
	_, err := oidc.NewVerifier(context.Background(), "https://x", oidc.VerifierConfig{})
	if err == nil || !strings.Contains(err.Error(), "client ID") {
		t.Errorf("got %v want client-ID error", err)
	}
}

func TestNewVerifier_DiscoveryFails(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := oidc.NewVerifier(context.Background(), srv.URL, oidc.VerifierConfig{ClientID: "c"})
	if err == nil || !strings.Contains(err.Error(), "discovery") {
		t.Errorf("got %v want discovery error", err)
	}
}

func TestNewVerifier_MissingJWKSURI(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": "x"}) // no jwks_uri
	}))
	defer srv.Close()
	_, err := oidc.NewVerifier(context.Background(), srv.URL, oidc.VerifierConfig{ClientID: "c"})
	if err == nil || !strings.Contains(err.Error(), "jwks_uri") {
		t.Errorf("got %v want jwks_uri error", err)
	}
}

func TestVerifier_Verify_Success(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)

	v, err := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	tok := kit.signRS256(t, kit.kid, map[string]any{
		"iss":            kit.issuer(),
		"sub":            "user-1",
		"aud":            "cid",
		"exp":            float64(time.Now().Add(time.Hour).Unix()),
		"iat":            float64(time.Now().Unix()),
		"nonce":          "n",
		"email":          "u@example.com",
		"email_verified": true,
		"name":           "User",
	})

	idt, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if idt.Subject != "user-1" || idt.Issuer != kit.issuer() {
		t.Errorf("claims wrong: %+v", idt)
	}
	if !containsStr(idt.Audience, "cid") {
		t.Errorf("aud: %v", idt.Audience)
	}

	ui := idt.ToUserInfo()
	if ui.Email != "u@example.com" || ui.Name != "User" || !ui.EmailVerified {
		t.Errorf("UserInfo wrong: %+v", ui)
	}

	// DiscoveryEndpoints reflects the well-known doc.
	ep := v.DiscoveryEndpoints()
	if !strings.HasSuffix(ep.Token, "/token") || !strings.HasSuffix(ep.JWKS, "/jwks") {
		t.Errorf("endpoints: %+v", ep)
	}
}

// Audience can also be a JSON array; "aud":["cid","x"].

func TestVerifier_Verify_ArrayAudience(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})

	tok := kit.signRS256(t, kit.kid, map[string]any{
		"iss": kit.issuer(),
		"sub": "u",
		"aud": []any{"other", "cid"},
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("Verify with array aud: %v", err)
	}
}

func TestVerifier_Verify_BadSignature(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})

	tok := kit.signRS256(t, kit.kid, map[string]any{
		"iss": kit.issuer(), "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	parts := strings.Split(tok, ".")
	parts[2] = base64.RawURLEncoding.EncodeToString([]byte("bogus-signature-bytes"))
	bad := strings.Join(parts, ".")

	if _, err := v.Verify(context.Background(), bad); err == nil {
		t.Fatal("expected signature error")
	}
}

func TestVerifier_Verify_ExpiredToken(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})

	tok := kit.signRS256(t, kit.kid, map[string]any{
		"iss": kit.issuer(), "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(-time.Hour).Unix()),
	})
	_, err := v.Verify(context.Background(), tok)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("got %v want expired", err)
	}
}

func TestVerifier_Verify_AudienceMismatch(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})

	tok := kit.signRS256(t, kit.kid, map[string]any{
		"iss": kit.issuer(), "sub": "u", "aud": "other",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	_, err := v.Verify(context.Background(), tok)
	if err == nil || !strings.Contains(err.Error(), "audience") {
		t.Errorf("got %v want audience error", err)
	}
}

func TestVerifier_Verify_IssuerMismatch(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})

	tok := kit.signRS256(t, kit.kid, map[string]any{
		"iss": "https://wrong",
		"sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	_, err := v.Verify(context.Background(), tok)
	if err == nil || !strings.Contains(err.Error(), "issuer") {
		t.Errorf("got %v want issuer error", err)
	}
}

func TestVerifier_Verify_SkipIssuerCheck(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{
		ClientID:        "cid",
		SkipIssuerCheck: true,
	})
	tok := kit.signRS256(t, kit.kid, map[string]any{
		"iss": "https://wrong", "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Errorf("SkipIssuerCheck should allow mismatch: %v", err)
	}
}

func TestVerifier_Verify_UnsupportedAlg(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})

	// Manually craft an HS256-headed token (alg JWT-shaped only).
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT","kid":"x"}`))
	pl := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"u"}`))
	tok := hdr + "." + pl + ".sig"

	_, err := v.Verify(context.Background(), tok)
	if err == nil || !strings.Contains(err.Error(), "unsupported signing algorithm") {
		t.Errorf("got %v want unsupported-alg error", err)
	}
}

// Token signed with RS256 but JWK declares ES256 → alg-confusion defense should reject.

func TestVerifier_Verify_AlgMismatchWithJWK(t *testing.T) {
	t.Parallel()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.Gen: %v", err)
	}

	mux := http.NewServeMux()
	var srvURL string
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   srvURL,
			"jwks_uri": srvURL + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		nB := base64.RawURLEncoding.EncodeToString(priv.N.Bytes())
		eB := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes())
		// Lie about alg → say ES256 even though it's an RSA key.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA", "kid": "k1", "alg": "ES256", "use": "sig",
				"n": nB, "e": eB,
			}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	v, err := oidc.NewVerifier(context.Background(), srvURL, oidc.VerifierConfig{
		ClientID:             "cid",
		SupportedSigningAlgs: []string{"RS256", "ES256"},
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	// Sign with RS256 (matches what we have a key for) but JWK says ES256.
	hdr, _ := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT", "kid": "k1"})
	pl, _ := json.Marshal(map[string]any{
		"iss": srvURL, "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	signingInput := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(pl)
	h := sha256.Sum256([]byte(signingInput))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, h[:])
	tok := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)

	_, err = v.Verify(context.Background(), tok)
	if err == nil || !strings.Contains(err.Error(), "does not match JWK alg") {
		t.Errorf("got %v want alg-mismatch error", err)
	}
}

func TestVerifier_Verify_KIDNotInJWKS(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})

	tok := kit.signRS256(t, "missing-kid", map[string]any{
		"iss": kit.issuer(), "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	_, err := v.Verify(context.Background(), tok)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("got %v want kid-not-found error", err)
	}
}

func TestVerifier_KIDMissRefreshIsThrottled(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{
		ClientID:          "cid",
		JWKSCacheDuration: time.Hour,
	})

	warmup := kit.signRS256(t, kit.kid, map[string]any{
		"iss": kit.issuer(), "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	if _, err := v.Verify(context.Background(), warmup); err != nil {
		t.Fatalf("warmup Verify: %v", err)
	}

	missing := kit.signRS256(t, "missing-kid", map[string]any{
		"iss": kit.issuer(), "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	if _, err := v.Verify(context.Background(), missing); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("first missing Verify: got %v want kid-not-found error", err)
	}
	hitsAfterFirstMiss := kit.hits.Load()

	if _, err := v.Verify(context.Background(), missing); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("second missing Verify: got %v want kid-not-found error", err)
	}
	if got := kit.hits.Load(); got != hitsAfterFirstMiss {
		t.Fatalf("expected throttled JWKS refresh after miss: hits %d -> %d", hitsAfterFirstMiss, got)
	}
}

func TestVerifier_Verify_MalformedToken(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{ClientID: "cid"})
	_, err := v.Verify(context.Background(), "no-dots-here")
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Errorf("got %v want malformed error", err)
	}
}

// JWKS cache: a second Verify (with a different kid in the same JWKS) should
// not refetch JWKS within the cache window.

func TestVerifier_JWKSCached(t *testing.T) {
	t.Parallel()
	kit := newRSATestKit(t)
	v, _ := oidc.NewVerifier(context.Background(), kit.issuer(), oidc.VerifierConfig{
		ClientID:          "cid",
		JWKSCacheDuration: time.Hour,
	})
	tok := kit.signRS256(t, kit.kid, map[string]any{
		"iss": kit.issuer(), "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})

	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("first Verify: %v", err)
	}
	hits1 := kit.hits.Load()
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("second Verify: %v", err)
	}
	hits2 := kit.hits.Load()
	if hits2 != hits1 {
		t.Errorf("JWKS should be cached: hits %d -> %d", hits1, hits2)
	}
}

// DiscoveryEndpoints on a zero-value Verifier returns zero struct.
// (Indirectly exercises the nil-disco path.)

func TestVerifier_DiscoveryEndpoints_NilWhenUndiscovered(t *testing.T) {
	t.Parallel()
	v := &oidc.Verifier{}
	ep := v.DiscoveryEndpoints()
	if ep.Authorization != "" || ep.Token != "" || ep.JWKS != "" || ep.UserInfo != "" {
		t.Errorf("expected zero endpoints, got %+v", ep)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// EC key path — ensures ecPublicKey + verifyECDSA + curves are exercised.
// ─────────────────────────────────────────────────────────────────────────────

func TestVerifier_Verify_ES256(t *testing.T) {
	t.Parallel()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ec gen: %v", err)
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
		// Avoid the deprecated ecdsa.PublicKey.X/Y fields by exporting via
		// crypto/ecdh's uncompressed point encoding (0x04 || X || Y).
		ecdhPub, ecdhErr := priv.PublicKey.ECDH()
		if ecdhErr != nil {
			t.Fatalf("ECDH conversion: %v", ecdhErr)
		}
		raw := ecdhPub.Bytes() // 65 bytes for P-256
		xB := base64.RawURLEncoding.EncodeToString(raw[1:33])
		yB := base64.RawURLEncoding.EncodeToString(raw[33:65])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "EC", "kid": "ec1", "alg": "ES256", "use": "sig",
				"crv": "P-256", "x": xB, "y": yB,
			}},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	srvURL = srv.URL

	v, err := oidc.NewVerifier(context.Background(), srvURL, oidc.VerifierConfig{
		ClientID:             "cid",
		SupportedSigningAlgs: []string{"ES256"},
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	// Build + sign ES256.
	hdr, _ := json.Marshal(map[string]any{"alg": "ES256", "typ": "JWT", "kid": "ec1"})
	pl, _ := json.Marshal(map[string]any{
		"iss": srvURL, "sub": "u", "aud": "cid",
		"exp": float64(time.Now().Add(time.Hour).Unix()),
	})
	signingInput := base64.RawURLEncoding.EncodeToString(hdr) + "." + base64.RawURLEncoding.EncodeToString(pl)
	h := sha256.Sum256([]byte(signingInput))
	sig, err := ecdsa.SignASN1(rand.Reader, priv, h[:])
	if err != nil {
		t.Fatalf("ec sign: %v", err)
	}
	tok := signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)

	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("ES256 verify: %v", err)
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────

func containsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// silence unused-import warnings if any helper is removed during edits.
var _ = fmt.Sprintf
