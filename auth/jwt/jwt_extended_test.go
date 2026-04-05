package jwt

import (
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// ── Algorithm Mismatch Attack ───────────────────────────────────────

func TestParse_AlgorithmMismatch_HS256TokenWithHS512Validator(t *testing.T) {
	cfg256 := newTestConfig()
	cfg256.Method = HS256
	svc256, _ := NewService(cfg256, func() *testClaims { return &testClaims{} })

	cfg512 := newTestConfig()
	cfg512.Method = HS512
	svc512, _ := NewService(cfg512, func() *testClaims { return &testClaims{} })

	token, _ := svc256.GenerateAccess(&testClaims{UserID: "user-1"})
	_, err := svc512.Parse(token)
	if err == nil {
		t.Fatal("expected error when algorithm mismatches (HS256 token validated with HS512)")
	}
}

func TestParse_UnsupportedAlgorithm(t *testing.T) {
	cfg := &Config{Method: "INVALID"}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected error for unsupported signing method")
	}
}

// ── Token Format Attacks ────────────────────────────────────────────

func TestParse_InvalidTokenFormats(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := NewService(cfg, func() *testClaims { return &testClaims{} })

	tests := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"no dots", "nodots"},
		{"one dot", "header.payload"},
		{"empty segments", ".."},
		{"whitespace", "   "},
		{"just dots", "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Parse(tt.token)
			if err == nil {
				t.Errorf("expected error for token format: %q", tt.token)
			}
		})
	}
}

// ── Expiry Edge Cases ───────────────────────────────────────────────

func TestParse_ExpiredToken(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := NewService(cfg, func() *testClaims { return &testClaims{} })

	claims := &testClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
			IssuedAt:  gojwt.NewNumericDate(time.Now().Add(-2 * time.Minute)),
		},
		UserID: "user-1",
	}
	token, err := svc.Generate(claims)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	_, err = svc.Parse(token)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

// ── Config Validation ───────────────────────────────────────────────

func TestConfig_MissingSecret(t *testing.T) {
	cfg := &Config{Method: HS256, Secret: ""}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected error for missing HMAC secret")
	}
}

func TestConfig_RSARequiresKey(t *testing.T) {
	cfg := &Config{Method: RS256}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected error for RS256 without key")
	}
}

func TestConfig_ESRequiresKey(t *testing.T) {
	cfg := &Config{Method: ES256}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected error for ES256 without key")
	}
}

func TestConfig_ApplyDefaultsTTL(t *testing.T) {
	cfg := &Config{Secret: "s", Method: HS256}
	cfg.ApplyDefaults()
	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Errorf("default access TTL should be 15m, got %v", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 7*24*time.Hour {
		t.Errorf("default refresh TTL should be 7d, got %v", cfg.RefreshTokenTTL)
	}
}

// ── Claims Edge Cases ───────────────────────────────────────────────

func TestGenerate_EmptyClaims(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := NewService(cfg, func() *testClaims { return &testClaims{} })

	claims := &testClaims{} // no fields set
	token, err := svc.GenerateAccess(claims)
	if err != nil {
		t.Fatalf("GenerateAccess with empty claims: %v", err)
	}
	parsed, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse empty claims token: %v", err)
	}
	if parsed.UserID != "" {
		t.Errorf("expected empty UserID, got %q", parsed.UserID)
	}
}

func TestGenerate_LargeClaims(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := NewService(cfg, func() *testClaims { return &testClaims{} })

	claims := &testClaims{
		UserID: strings.Repeat("x", 10000),
		Email:  strings.Repeat("e", 10000),
	}
	token, err := svc.GenerateAccess(claims)
	if err != nil {
		t.Fatalf("GenerateAccess with large claims: %v", err)
	}
	parsed, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse large claims: %v", err)
	}
	if len(parsed.UserID) != 10000 {
		t.Errorf("large UserID not preserved, got len=%d", len(parsed.UserID))
	}
}

func TestGenerate_SpecialCharactersInClaims(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := NewService(cfg, func() *testClaims { return &testClaims{} })

	claims := &testClaims{
		UserID: "user/with\"special<chars>&",
		Email:  "用户@例子.中国",
	}
	token, err := svc.GenerateAccess(claims)
	if err != nil {
		t.Fatalf("GenerateAccess: %v", err)
	}
	parsed, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Email != claims.Email {
		t.Errorf("special chars not preserved: got %q", parsed.Email)
	}
}

// ── Wrong Audience ──────────────────────────────────────────────────

func TestParse_WrongAudience(t *testing.T) {
	cfgGen := newTestConfig()
	cfgGen.Audience = []string{"aud-a"}
	svcGen, _ := NewService(cfgGen, func() *testClaims { return &testClaims{} })

	cfgVal := newTestConfig()
	cfgVal.Audience = []string{"aud-b"}
	svcVal, _ := NewService(cfgVal, func() *testClaims { return &testClaims{} })

	token, _ := svcGen.GenerateAccess(&testClaims{UserID: "u"})
	_, err := svcVal.Parse(token)
	if err == nil {
		t.Fatal("expected error when audience mismatches")
	}
}

// ── No Error Leaks ──────────────────────────────────────────────────

func TestParse_ErrorDoesNotLeakSecret(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := NewService(cfg, func() *testClaims { return &testClaims{} })

	_, err := svc.Parse("invalid.token.string")
	if err != nil && strings.Contains(err.Error(), cfg.Secret) {
		t.Error("error message should not contain the secret key")
	}
}
