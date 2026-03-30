package jwt

import (
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// testClaims is a custom claims type that embeds RegisteredClaims
// (the common pattern) WITHOUT implementing SetDefaults.
type testClaims struct {
	gojwt.RegisteredClaims
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

// testClaimsWithDefaults implements the SetDefaults interface.
type testClaimsWithDefaults struct {
	gojwt.RegisteredClaims
	UserID        string `json:"user_id"`
	defaultsCalled bool
}

func (c *testClaimsWithDefaults) SetDefaults(now time.Time, ttl time.Duration, issuer string, audience []string) {
	c.defaultsCalled = true
	if c.IssuedAt == nil {
		c.IssuedAt = gojwt.NewNumericDate(now)
	}
	if c.ExpiresAt == nil && ttl > 0 {
		c.ExpiresAt = gojwt.NewNumericDate(now.Add(ttl))
	}
	if c.Issuer == "" && issuer != "" {
		c.Issuer = issuer
	}
	if len(c.Audience) == 0 && len(audience) > 0 {
		c.Audience = gojwt.ClaimStrings(audience)
	}
}

func newTestConfig() *Config {
	return &Config{
		Secret:          "test-secret-key-that-is-long-enough",
		Method:          HS256,
		Issuer:          "test-issuer",
		Audience:        []string{"test-audience"},
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 7 * 24 * time.Hour,
	}
}

func TestNewService(t *testing.T) {
	cfg := newTestConfig()
	svc, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestNewService_MissingSecret(t *testing.T) {
	cfg := &Config{Method: HS256}
	_, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err == nil {
		t.Fatal("expected error for missing secret")
	}
}

func TestGenerateAccess_SetsClaimsViaReflection(t *testing.T) {
	cfg := newTestConfig()
	svc, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	claims := &testClaims{UserID: "user-123", Email: "test@example.com"}
	token, err := svc.GenerateAccess(claims)
	if err != nil {
		t.Fatalf("GenerateAccess: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	// Verify claims were set
	if claims.Issuer != "test-issuer" {
		t.Errorf("expected issuer 'test-issuer', got '%s'", claims.Issuer)
	}
	if len(claims.Audience) == 0 || claims.Audience[0] != "test-audience" {
		t.Errorf("expected audience ['test-audience'], got %v", claims.Audience)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	if claims.IssuedAt == nil {
		t.Fatal("expected IssuedAt to be set")
	}
}

func TestGenerateAccess_SetsClaimsViaSetDefaults(t *testing.T) {
	cfg := newTestConfig()
	svc, err := NewService(cfg, func() *testClaimsWithDefaults { return &testClaimsWithDefaults{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	claims := &testClaimsWithDefaults{UserID: "user-456"}
	token, err := svc.GenerateAccess(claims)
	if err != nil {
		t.Fatalf("GenerateAccess: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if !claims.defaultsCalled {
		t.Fatal("expected SetDefaults to be called")
	}
	if claims.Issuer != "test-issuer" {
		t.Errorf("expected issuer 'test-issuer', got '%s'", claims.Issuer)
	}
}

func TestRoundTrip_GenerateAndParse(t *testing.T) {
	cfg := newTestConfig()
	svc, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	original := &testClaims{UserID: "user-789", Email: "round@trip.com"}
	token, err := svc.GenerateAccess(original)
	if err != nil {
		t.Fatalf("GenerateAccess: %v", err)
	}

	parsed, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if parsed.UserID != "user-789" {
		t.Errorf("expected UserID 'user-789', got '%s'", parsed.UserID)
	}
	if parsed.Email != "round@trip.com" {
		t.Errorf("expected Email 'round@trip.com', got '%s'", parsed.Email)
	}
	if parsed.Issuer != "test-issuer" {
		t.Errorf("expected Issuer 'test-issuer', got '%s'", parsed.Issuer)
	}
	if len(parsed.Audience) == 0 || parsed.Audience[0] != "test-audience" {
		t.Errorf("expected Audience ['test-audience'], got %v", parsed.Audience)
	}
}

func TestParse_InvalidToken(t *testing.T) {
	cfg := newTestConfig()
	svc, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.Parse("invalid-token-string")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestParse_WrongSecret(t *testing.T) {
	cfg1 := newTestConfig()
	svc1, _ := NewService(cfg1, func() *testClaims { return &testClaims{} })

	cfg2 := newTestConfig()
	cfg2.Secret = "different-secret-key-that-is-also-long"
	svc2, _ := NewService(cfg2, func() *testClaims { return &testClaims{} })

	token, _ := svc1.GenerateAccess(&testClaims{UserID: "user-1"})
	_, err := svc2.Parse(token)
	if err == nil {
		t.Fatal("expected error when parsing with wrong secret")
	}
}

func TestParse_WrongIssuer(t *testing.T) {
	cfg1 := newTestConfig()
	svc1, _ := NewService(cfg1, func() *testClaims { return &testClaims{} })

	cfg2 := newTestConfig()
	cfg2.Issuer = "wrong-issuer"
	svc2, _ := NewService(cfg2, func() *testClaims { return &testClaims{} })

	token, _ := svc1.GenerateAccess(&testClaims{UserID: "user-1"})
	_, err := svc2.Parse(token)
	if err == nil {
		t.Fatal("expected error when issuer doesn't match")
	}
}

func TestGenerateRefresh_UsesRefreshTTL(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := NewService(cfg, func() *testClaims { return &testClaims{} })

	claims := &testClaims{UserID: "user-1"}
	before := time.Now()
	_, err := svc.GenerateRefresh(claims)
	if err != nil {
		t.Fatalf("GenerateRefresh: %v", err)
	}

	expected := before.Add(7 * 24 * time.Hour)
	if claims.ExpiresAt.Time.Before(expected.Add(-time.Second)) {
		t.Error("refresh token TTL seems too short")
	}
}

func TestFindRegisteredClaims_EmbeddedStruct(t *testing.T) {
	claims := &testClaims{UserID: "test"}
	rc := findRegisteredClaims(claims)
	if rc == nil {
		t.Fatal("expected to find RegisteredClaims in embedded struct")
	}
	rc.Issuer = "set-via-reflection"
	if claims.Issuer != "set-via-reflection" {
		t.Error("setting via reflected pointer should modify original")
	}
}

func TestFindRegisteredClaims_Nil(t *testing.T) {
	rc := findRegisteredClaims(nil)
	if rc != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestFindRegisteredClaims_NonStruct(t *testing.T) {
	s := "not a struct"
	rc := findRegisteredClaims(&s)
	if rc != nil {
		t.Fatal("expected nil for non-struct")
	}
}

func TestValidatorFunc(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := NewService(cfg, func() *testClaims { return &testClaims{} })

	token, _ := svc.GenerateAccess(&testClaims{UserID: "user-1"})
	validator := svc.ValidatorFunc()

	result, err := validator(token)
	if err != nil {
		t.Fatalf("ValidatorFunc: %v", err)
	}
	parsed, ok := result.(*testClaims)
	if !ok {
		t.Fatal("expected *testClaims from ValidatorFunc")
	}
	if parsed.UserID != "user-1" {
		t.Errorf("expected UserID 'user-1', got '%s'", parsed.UserID)
	}
}
