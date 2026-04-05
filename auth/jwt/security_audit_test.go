package jwt_test

import (
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	"github.com/kbukum/gokit/auth/jwt"
)

// ─── Security Audit: JWT Algorithm Confusion Prevention ─────────────────────

type AuditClaims struct {
	gojwt.RegisteredClaims
	UserID string `json:"user_id"`
}

func TestSecurityAudit_AlgorithmConfusion_HS256RejectedByHS512(t *testing.T) {
	t.Parallel()

	svc256, err := jwt.NewService(&jwt.Config{
		Secret: "shared-secret-key-for-audit-test",
		Method: jwt.HS256,
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("failed to create HS256 service: %v", err)
	}

	claims := &AuditClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   "user-123",
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		UserID: "user-123",
	}

	token, err := svc256.Generate(claims)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	svc512, err := jwt.NewService(&jwt.Config{
		Secret: "shared-secret-key-for-audit-test",
		Method: jwt.HS512,
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("failed to create HS512 service: %v", err)
	}

	_, err = svc512.Parse(token)
	if err == nil {
		t.Error("HS256 token was accepted by HS512 service — algorithm confusion vulnerability")
	}
}

func TestSecurityAudit_NoneAlgorithm_Rejected(t *testing.T) {
	t.Parallel()

	svc, err := jwt.NewService(&jwt.Config{
		Secret: "test-secret-for-none-algorithm1",
		Method: jwt.HS256,
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	noneToken := gojwt.NewWithClaims(gojwt.SigningMethodNone, &AuditClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	tokenStr, _ := noneToken.SignedString(gojwt.UnsafeAllowNoneSignatureType)

	_, err = svc.Parse(tokenStr)
	if err == nil {
		t.Error("token with 'none' algorithm was accepted — CRITICAL vulnerability")
	}
}

func TestSecurityAudit_EmptySecret_Rejected(t *testing.T) {
	t.Parallel()

	_, err := jwt.NewService(&jwt.Config{
		Secret: "",
		Method: jwt.HS256,
	}, func() *AuditClaims { return &AuditClaims{} })
	if err == nil {
		t.Error("JWT service should reject empty HMAC secret")
	}
}

func TestSecurityAudit_ParseError_DoesNotLeakSecret(t *testing.T) {
	t.Parallel()

	secret := "super-secret-jwt-key-must-not-leak-in-errors"
	svc, err := jwt.NewService(&jwt.Config{
		Secret: secret,
		Method: jwt.HS256,
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.Parse("invalid.token.here")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("JWT parse error leaked secret: %s", err.Error())
	}
}

func TestSecurityAudit_MalformedTokens_ReturnError(t *testing.T) {
	t.Parallel()

	svc, err := jwt.NewService(&jwt.Config{
		Secret: "test-secret-for-malformed-tokens",
		Method: jwt.HS256,
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	tokens := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"single segment", "aaa"},
		{"two segments", "aaa.bbb"},
		{"unicode garbage", "こんにちは.世界.テスト"},
		{"spaces", "   "},
		{"very long", strings.Repeat("a", 100000)},
		{"null bytes", "aaa\x00bbb.ccc\x00ddd.eee\x00fff"},
	}

	for _, tc := range tokens {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := svc.Parse(tc.token)
			if err == nil {
				t.Error("expected error for malformed token")
			}
		})
	}
}

func TestSecurityAudit_ConcurrentParse_NoRace(t *testing.T) {
	t.Parallel()

	svc, err := jwt.NewService(&jwt.Config{
		Secret: "concurrent-test-secret-key-here",
		Method: jwt.HS256,
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	claims := &AuditClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := svc.Generate(claims)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func() {
			_, _ = svc.Parse(token)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}
