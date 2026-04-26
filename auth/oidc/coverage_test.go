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

// ─────────────────────────────────────────────────────────────────────────────
// Config
// ─────────────────────────────────────────────────────────────────────────────

func TestConfig_ApplyDefaults(t *testing.T) {
	t.Parallel()
	c := oidc.Config{}
	c.ApplyDefaults()
	if want := []string{"openid", "email", "profile"}; !equalStrings(c.Scopes, want) {
		t.Errorf("Scopes default: got %v want %v", c.Scopes, want)
	}
	if want := []string{"RS256"}; !equalStrings(c.SupportedSigningAlgs, want) {
		t.Errorf("SupportedSigningAlgs default: got %v want %v", c.SupportedSigningAlgs, want)
	}
	if c.JWKSCacheDuration != time.Hour {
		t.Errorf("JWKSCacheDuration default: got %v want 1h", c.JWKSCacheDuration)
	}
	if c.HTTPTimeout != 10*time.Second {
		t.Errorf("HTTPTimeout default: got %v want 10s", c.HTTPTimeout)
	}
}

func TestConfig_ApplyDefaults_DoesNotOverrideExplicit(t *testing.T) {
	t.Parallel()
	c := oidc.Config{
		Scopes:               []string{"x"},
		SupportedSigningAlgs: []string{"ES256"},
		JWKSCacheDuration:    5 * time.Minute,
		HTTPTimeout:          1 * time.Second,
	}
	c.ApplyDefaults()
	if !equalStrings(c.Scopes, []string{"x"}) {
		t.Errorf("Scopes overridden: %v", c.Scopes)
	}
	if !equalStrings(c.SupportedSigningAlgs, []string{"ES256"}) {
		t.Errorf("Algs overridden: %v", c.SupportedSigningAlgs)
	}
	if c.JWKSCacheDuration != 5*time.Minute {
		t.Errorf("Cache duration overridden")
	}
	if c.HTTPTimeout != 1*time.Second {
		t.Errorf("HTTP timeout overridden")
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		c    oidc.Config
		ok   bool
		msg  string
	}{
		{"disabled-no-issuer-ok", oidc.Config{Enabled: false}, true, ""},
		{"missing-issuer", oidc.Config{Enabled: true, ClientID: "c"}, false, "issuer"},
		{"missing-client-id", oidc.Config{Enabled: true, Issuer: "https://x"}, false, "client_id"},
		{"valid", oidc.Config{Enabled: true, Issuer: "https://x", ClientID: "c"}, true, ""},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.c.Validate()
			if tc.ok && err != nil {
				t.Fatalf("expected ok, got %v", err)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.msg)
				}
				if !strings.Contains(err.Error(), tc.msg) {
					t.Errorf("error %v does not contain %q", err, tc.msg)
				}
			}
		})
	}
}

func TestConfig_ToVerifierConfig(t *testing.T) {
	t.Parallel()
	c := oidc.Config{
		ClientID:             "cid",
		SupportedSigningAlgs: []string{"RS256", "ES256"},
		JWKSCacheDuration:    7 * time.Minute,
		SkipIssuerCheck:      true,
	}
	v := c.ToVerifierConfig()
	if v.ClientID != "cid" {
		t.Errorf("ClientID: got %q", v.ClientID)
	}
	if !equalStrings(v.SupportedSigningAlgs, []string{"RS256", "ES256"}) {
		t.Errorf("Algs: got %v", v.SupportedSigningAlgs)
	}
	if v.JWKSCacheDuration != 7*time.Minute {
		t.Errorf("Cache: got %v", v.JWKSCacheDuration)
	}
	if !v.SkipIssuerCheck {
		t.Errorf("SkipIssuerCheck not propagated")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// state.go: GenerateState / NewPKCE / GenerateNonce
// ─────────────────────────────────────────────────────────────────────────────

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
		nB := base64.RawURLEncoding.EncodeToString(priv.PublicKey.N.Bytes())
		eBig := big.NewInt(int64(priv.PublicKey.E))
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
		nB := base64.RawURLEncoding.EncodeToString(priv.PublicKey.N.Bytes())
		eB := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.PublicKey.E)).Bytes())
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
		xB := base64.RawURLEncoding.EncodeToString(priv.PublicKey.X.Bytes())
		yB := base64.RawURLEncoding.EncodeToString(priv.PublicKey.Y.Bytes())
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

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

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
