package jwt_test

import (
	"crypto/rand"
	"crypto/rsa"
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

func TestSecurityAudit_DifferentHMACSecret_RejectsToken(t *testing.T) {
	t.Parallel()

	svcA, err := jwt.NewService(&jwt.Config{
		Secret:             "shared-secret-key-for-audit-test-123",
		Method:             jwt.HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("failed to create service A: %v", err)
	}

	claims := &AuditClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject: "user-123",
		},
		UserID: "user-123",
	}

	// GenerateAccess fills in iss, aud, iat, nbf, exp so the token is well-formed.
	// The only reason svcB should reject it is the HMAC signature mismatch.
	token, err := svcA.GenerateAccess(claims)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	svcB, err := jwt.NewService(&jwt.Config{
		Secret:             "different-shared-secret-key-for-audit",
		Method:             jwt.HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("failed to create service B: %v", err)
	}

	_, err = svcB.Parse(token)
	if err == nil {
		t.Error("token signed with service A's HMAC secret was accepted by service B — key isolation failure")
	}
}

func TestSecurityAudit_NoneAlgorithm_Rejected(t *testing.T) {
	t.Parallel()

	svc, err := jwt.NewService(&jwt.Config{
		Secret:             "test-secret-for-none-algorithm-1234",
		Method:             jwt.HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
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
		Secret:             "",
		Method:             jwt.HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
	}, func() *AuditClaims { return &AuditClaims{} })
	if err == nil {
		t.Error("JWT service should reject empty HMAC secret")
	}
}

func TestSecurityAudit_ParseError_DoesNotLeakSecret(t *testing.T) {
	t.Parallel()

	secret := "super-secret-jwt-key-must-not-leak-in-errors"
	svc, err := jwt.NewService(&jwt.Config{
		Secret:             secret,
		Method:             jwt.HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
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
		Secret:             "test-secret-for-malformed-tokens",
		Method:             jwt.HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
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
		Secret:             "concurrent-test-secret-key-here1",
		Method:             jwt.HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
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

// TestSecurityAudit_AlgorithmConfusion_HS256TokenRejectedByRS256Verifier verifies
// that a token signed with HS256 is rejected when presented to an RS256 service.
// This is the classic algorithm-confusion attack: a public key (treated as an HMAC
// secret by the attacker) is used to forge HS256 signatures that an RS256 verifier
// would accept if alg-from-header trust were used. WithValidMethods prevents this.
func TestSecurityAudit_AlgorithmConfusion_HS256TokenRejectedByRS256Verifier(t *testing.T) {
	t.Parallel()

	// Build an RS256 service (signer + verifier).
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	rs256Svc, err := jwt.NewService(&jwt.Config{
		Method:     jwt.RS256,
		PrivateKey: rsaKey,
		Issuer:     "issuer",
		Audience:   []string{"aud"},
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("create RS256 service: %v", err)
	}

	// Build a separate HS256 service and generate a token with it.
	hs256Svc, err := jwt.NewService(&jwt.Config{
		Secret:             "shared-secret-for-confusion-test-123",
		Method:             jwt.HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
	}, func() *AuditClaims { return &AuditClaims{} })
	if err != nil {
		t.Fatalf("create HS256 service: %v", err)
	}

	hs256Token, err := hs256Svc.GenerateAccess(&AuditClaims{
		RegisteredClaims: gojwt.RegisteredClaims{
			Subject:   "attacker",
			ExpiresAt: gojwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	if err != nil {
		t.Fatalf("generate HS256 token: %v", err)
	}

	// The RS256 verifier must reject the HS256-signed token.
	_, err = rs256Svc.Parse(hs256Token)
	if err == nil {
		t.Error("CRITICAL: RS256 verifier accepted an HS256-signed token — algorithm confusion vulnerability")
	}
}
