package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

func TestParse_UnsupportedAlgorithm(t *testing.T) {
	cfg := &Config{Method: "INVALID", Issuer: "issuer", Audience: []string{"aud"}}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected error for unsupported signing method")
	}
}

func TestConfig_MissingSecret(t *testing.T) {
	cfg := &Config{
		Method:             HS256,
		Secret:             "",
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
	}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected error for missing HMAC secret")
	}
}

func TestConfig_RSARequiresKey(t *testing.T) {
	cfg := &Config{Method: RS256, Issuer: "issuer", Audience: []string{"aud"}}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected error for RS256 without key")
	}
}

func TestConfig_ESRequiresKey(t *testing.T) {
	cfg := &Config{Method: ES256, Issuer: "issuer", Audience: []string{"aud"}}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected error for ES256 without key")
	}
}

func TestConfig_ApplyDefaultsTTL(t *testing.T) {
	cfg := &Config{
		Secret:             "12345678901234567890123456789012",
		Method:             HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
	}
	cfg.ApplyDefaults()
	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Errorf("default access TTL should be 15m, got %v", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 7*24*time.Hour {
		t.Errorf("default refresh TTL should be 7d, got %v", cfg.RefreshTokenTTL)
	}
	if cfg.ClockSkew != 30*time.Second {
		t.Errorf("default clock skew should be 30s, got %v", cfg.ClockSkew)
	}
}

func TestConfigValidateRejectsMismatchedKeyTypes(t *testing.T) {
	t.Parallel()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa keygen: %v", err)
	}
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519 keygen: %v", err)
	}

	cases := map[string]*Config{
		"rs256-bad-private":   {Method: RS256, Issuer: "i", Audience: []string{"a"}, PrivateKey: ecKey},
		"rs256-bad-public":    {Method: RS256, Issuer: "i", Audience: []string{"a"}, PrivateKey: rsaKey, PublicKey: ecKey.Public()},
		"es256-bad-private":   {Method: ES256, Issuer: "i", Audience: []string{"a"}, PrivateKey: rsaKey},
		"es256-bad-public":    {Method: ES256, Issuer: "i", Audience: []string{"a"}, PrivateKey: ecKey, PublicKey: rsaKey.Public()},
		"eddsa-missing-key":   {Method: EdDSA, Issuer: "i", Audience: []string{"a"}},
		"eddsa-bad-private":   {Method: EdDSA, Issuer: "i", Audience: []string{"a"}, PrivateKey: rsaKey},
		"eddsa-short-private": {Method: EdDSA, Issuer: "i", Audience: []string{"a"}, PrivateKey: ed25519.PrivateKey("short")},
		"eddsa-bad-public":    {Method: EdDSA, Issuer: "i", Audience: []string{"a"}, PrivateKey: edPriv, PublicKey: rsaKey.Public()},
		"eddsa-short-public":  {Method: EdDSA, Issuer: "i", Audience: []string{"a"}, PrivateKey: edPriv, PublicKey: ed25519.PublicKey("short")},
	}
	_ = edPub
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := cfg.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestConfigValidateRejectsShortRefreshSecret(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Method:             HS256,
		AllowSymmetricHMAC: true,
		Secret:             strings.Repeat("s", 32),
		RefreshSecret:      "short",
		Issuer:             "i",
		Audience:           []string{"a"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected short refresh_secret rejection")
	}
}

func TestConfigValidateRejectsClockSkewBounds(t *testing.T) {
	t.Parallel()
	base := func() *Config {
		return &Config{Method: HS256, AllowSymmetricHMAC: true, Secret: strings.Repeat("s", 32), Issuer: "i", Audience: []string{"a"}}
	}
	neg := base()
	neg.ClockSkew = -1
	if err := neg.Validate(); err == nil {
		t.Fatal("expected negative clock_skew rejection")
	}
	over := base()
	over.ClockSkew = 2 * time.Minute
	if err := over.Validate(); err == nil {
		t.Fatal("expected over-max clock_skew rejection")
	}
}

func TestSigningMethodMapping(t *testing.T) {
	t.Parallel()
	cases := map[SigningMethod]string{HS256: "HS256", RS256: "RS256", ES256: "ES256", EdDSA: "EdDSA"}
	for method, alg := range cases {
		if got := (&Config{Method: method}).signingMethod().Alg(); got != alg {
			t.Fatalf("signingMethod(%s).Alg() = %q, want %q", method, got, alg)
		}
	}
	if got := (&Config{Method: "unknown"}).signingMethod(); got != gojwt.SigningMethodRS256 {
		t.Fatalf("unknown method should fall back to RS256, got %v", got)
	}
}

func TestSignKeyAndRefreshSignKey(t *testing.T) {
	t.Parallel()
	hmac := &Config{Method: HS256, Secret: "access-secret", RefreshSecret: "refresh-secret"}
	if string(hmac.signKey().([]byte)) != "access-secret" {
		t.Fatal("HS256 signKey should return access secret")
	}
	if string(hmac.refreshSignKey().([]byte)) != "refresh-secret" {
		t.Fatal("HS256 refreshSignKey should return refresh secret")
	}
	hmacNoRefresh := &Config{Method: HS256, Secret: "access-secret"}
	if string(hmacNoRefresh.refreshSignKey().([]byte)) != "access-secret" {
		t.Fatal("HS256 refreshSignKey without refresh secret should fall back to access secret")
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	asym := &Config{Method: RS256, PrivateKey: rsaKey}
	if asym.signKey() != any(rsaKey) {
		t.Fatal("RS256 signKey should return private key")
	}
	if asym.refreshSignKey() == nil {
		t.Fatal("RS256 refreshSignKey should reuse sign key")
	}
}

func TestVerifyKeyFallbacks(t *testing.T) {
	t.Parallel()
	// Unknown method falls back to HMAC secret bytes.
	if got := (&Config{Method: "unknown", Secret: "s"}).verifyKey().([]byte); string(got) != "s" {
		t.Fatalf("unknown verifyKey = %q, want s", got)
	}
	// Asymmetric methods without a usable typed private key return it verbatim.
	for _, method := range []SigningMethod{RS256, ES256, EdDSA} {
		cfg := &Config{Method: method, PrivateKey: "raw-key-material"}
		if got, ok := cfg.verifyKey().(string); !ok || got != "raw-key-material" {
			t.Fatalf("%s verifyKey fallback = %v, want raw-key-material", method, cfg.verifyKey())
		}
	}
	// refreshVerifyKey with HS256 refresh secret returns the refresh secret.
	hmac := &Config{Method: HS256, Secret: "a", RefreshSecret: "r"}
	if string(hmac.refreshVerifyKey().([]byte)) != "r" {
		t.Fatal("HS256 refreshVerifyKey should return refresh secret")
	}
}
