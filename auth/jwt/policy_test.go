package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

func TestConfig_HS256RequiresExplicitOptIn(t *testing.T) {
	cfg := &Config{
		Method:   HS256,
		Secret:   "12345678901234567890123456789012",
		Issuer:   "issuer",
		Audience: []string{"aud"},
	}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected HS256 without explicit opt-in to fail")
	}
}

func TestParse_MissingNotBeforeRejected(t *testing.T) {
	svc, err := NewService(newTestConfig(), func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	token, err := svc.Generate(&testClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Issuer:    "test-issuer",
			Audience:  []string{"test-audience"},
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  gojwt.NewNumericDate(time.Now()),
		},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	_, err = svc.Parse(token)
	if err == nil {
		t.Fatal("expected token missing nbf to be rejected")
	}
}

func TestEdDSA_RoundTrip(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	svc, err := NewService(&Config{
		Method:     EdDSA,
		PrivateKey: priv,
		PublicKey:  pub,
		Issuer:     "issuer",
		Audience:   []string{"aud"},
	}, func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	token, err := svc.GenerateAccess(&testClaims{UserID: "user-1"})
	if err != nil {
		t.Fatalf("GenerateAccess: %v", err)
	}

	parsed, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.UserID != "user-1" {
		t.Fatalf("unexpected user id: %s", parsed.UserID)
	}
}

func TestConfig_Validate_AsymmetricKeyTypes(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "RS256",
			cfg:  &Config{Method: RS256, PrivateKey: rsaKey, PublicKey: &rsaKey.PublicKey, Issuer: "issuer", Audience: []string{"aud"}},
		},
		{
			name: "ES256",
			cfg:  &Config{Method: ES256, PrivateKey: ecKey, PublicKey: &ecKey.PublicKey, Issuer: "issuer", Audience: []string{"aud"}},
		},
		{
			name: "EdDSA",
			cfg: func() *Config {
				pub, priv, err := ed25519.GenerateKey(rand.Reader)
				if err != nil {
					t.Fatalf("GenerateKey: %v", err)
				}
				return &Config{Method: EdDSA, PrivateKey: priv, PublicKey: pub, Issuer: "issuer", Audience: []string{"aud"}}
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestConfig_Validate_PublicKeyTypeMismatchRejected(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "RS256",
			cfg:  &Config{Method: RS256, PrivateKey: rsaKey, PublicKey: &ecKey.PublicKey, Issuer: "issuer", Audience: []string{"aud"}},
		},
		{
			name: "ES256",
			cfg:  &Config{Method: ES256, PrivateKey: ecKey, PublicKey: &rsaKey.PublicKey, Issuer: "issuer", Audience: []string{"aud"}},
		},
		{
			name: "EdDSA",
			cfg:  &Config{Method: EdDSA, PrivateKey: priv, PublicKey: &rsaKey.PublicKey, Issuer: "issuer", Audience: []string{"aud"}},
		},
		{
			name: "HS256 short refresh secret",
			cfg: &Config{
				Method:             HS256,
				Secret:             "12345678901234567890123456789012",
				RefreshSecret:      "short",
				AllowSymmetricHMAC: true,
				Issuer:             "issuer",
				Audience:           []string{"aud"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestConfig_Validate_ClockSkewCap(t *testing.T) {
	cfg := &Config{
		Method:             HS256,
		Secret:             "12345678901234567890123456789012",
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
		ClockSkew:          2 * time.Minute,
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected excessive clock skew to fail")
	}
}

func TestRS256_RoundTrip(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	svc, err := NewService(&Config{
		Method:     RS256,
		PrivateKey: key,
		Issuer:     "issuer",
		Audience:   []string{"aud"},
	}, func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	token, err := svc.GenerateAccess(&testClaims{UserID: "user-rs"})
	if err != nil {
		t.Fatalf("GenerateAccess: %v", err)
	}

	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.UserID != "user-rs" {
		t.Fatalf("unexpected user id: %s", claims.UserID)
	}
}

func TestConfig_VerifyKeyVariants(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "HS256",
			cfg: Config{
				Method:             HS256,
				Secret:             "12345678901234567890123456789012",
				AllowSymmetricHMAC: true,
				Issuer:             "issuer",
				Audience:           []string{"aud"},
			},
		},
		{
			name: "RS256",
			cfg: Config{
				Method:     RS256,
				PrivateKey: rsaKey,
				Issuer:     "issuer",
				Audience:   []string{"aud"},
			},
		},
		{
			name: "ES256",
			cfg: Config{
				Method:     ES256,
				PrivateKey: ecKey,
				Issuer:     "issuer",
				Audience:   []string{"aud"},
			},
		},
		{
			name: "EdDSA",
			cfg: Config{
				Method:     EdDSA,
				PrivateKey: priv,
				PublicKey:  pub,
				Issuer:     "issuer",
				Audience:   []string{"aud"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cfg.signKey() == nil {
				t.Fatal("expected sign key")
			}
			if tt.cfg.verifyKey() == nil {
				t.Fatal("expected verify key")
			}
		})
	}
}
