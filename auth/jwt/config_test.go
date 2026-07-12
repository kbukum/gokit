package jwt

import (
	"testing"
	"time"
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
