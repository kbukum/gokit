package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// jwtSeed builds a JWT-like compact serialization (header.payload.signature)
// from raw header/payload JSON so fuzz seeds actually resemble tokens and hit
// the intended parse paths. A signature of "" produces a trailing dot (the
// unsecured/alg=none shape).
func jwtSeed(header, payload, signature string) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(header)) + "." + enc([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString([]byte(signature))
}

// jwtHeaderOnly builds a truncated token that carries only the header segment.
func jwtHeaderOnly(header string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(header))
}

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
	UserID         string `json:"user_id"`
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
		Secret:             "test-secret-key-that-is-long-enough",
		Method:             HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "test-issuer",
		Audience:           []string{"test-audience"},
		AccessTokenTTL:     15 * time.Minute,
		RefreshTokenTTL:    7 * 24 * time.Hour,
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
	cfg := &Config{
		Method:             HS256,
		AllowSymmetricHMAC: true,
		Issuer:             "issuer",
		Audience:           []string{"aud"},
	}
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
	if claims.ExpiresAt.Before(expected.Add(-time.Second)) {
		t.Error("refresh token TTL seems too short")
	}
}

func TestGenerateRefresh_UsesRefreshSecret(t *testing.T) {
	cfg := newTestConfig()
	cfg.RefreshSecret = "refresh-secret-key-that-is-long-enough"
	svc, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	token, err := svc.GenerateRefresh(&testClaims{UserID: "user-1"})
	if err != nil {
		t.Fatalf("GenerateRefresh: %v", err)
	}

	if _, err := svc.Parse(token); err == nil {
		t.Fatal("expected access-token parser to reject refresh token signed with refresh secret")
	}
	if _, err := svc.ParseRefresh(token); err != nil {
		t.Fatalf("ParseRefresh: %v", err)
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

func TestParse_DifferentHMACVerifierSecretRejected(t *testing.T) {
	cfg256 := newTestConfig()
	svc256, _ := NewService(cfg256, func() *testClaims { return &testClaims{} })

	token, _ := svc256.GenerateAccess(&testClaims{UserID: "user-1"})
	cfgMismatch := newTestConfig()
	cfgMismatch.Secret = "different-test-secret-key-that-is-long-enough"
	svcMismatch, _ := NewService(cfgMismatch, func() *testClaims { return &testClaims{} })
	_, err := svcMismatch.Parse(token)
	if err == nil {
		t.Fatal("expected error when token is parsed with a different verifier secret")
	}
}

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

func TestParse_ErrorDoesNotLeakSecret(t *testing.T) {
	cfg := newTestConfig()
	svc, _ := NewService(cfg, func() *testClaims { return &testClaims{} })

	_, err := svc.Parse("invalid.token.string")
	if err != nil && strings.Contains(err.Error(), cfg.Secret) {
		t.Error("error message should not contain the secret key")
	}
}

func asymmetricConfig(t *testing.T, method SigningMethod, withPublic bool) *Config {
	t.Helper()
	cfg := &Config{
		Method:          method,
		Issuer:          "test-issuer",
		Audience:        []string{"test-audience"},
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: time.Hour,
	}
	switch method {
	case RS256:
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("rsa keygen: %v", err)
		}
		cfg.PrivateKey = key
		if withPublic {
			cfg.PublicKey = &key.PublicKey
		}
	case ES256:
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("ecdsa keygen: %v", err)
		}
		cfg.PrivateKey = key
		if withPublic {
			cfg.PublicKey = &key.PublicKey
		}
	case EdDSA:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("ed25519 keygen: %v", err)
		}
		cfg.PrivateKey = priv
		if withPublic {
			cfg.PublicKey = pub
		}
	case HS256:
	}
	return cfg
}

func TestRoundTrip_AsymmetricMethods(t *testing.T) {
	t.Parallel()
	for _, method := range []SigningMethod{RS256, ES256, EdDSA} {
		for _, withPublic := range []bool{false, true} {
			name := string(method)
			if withPublic {
				name += "-explicit-public"
			} else {
				name += "-derived-public"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				cfg := asymmetricConfig(t, method, withPublic)
				svc, err := NewService(cfg, func() *testClaims { return &testClaims{} })
				if err != nil {
					t.Fatalf("NewService: %v", err)
				}
				access, err := svc.GenerateAccess(&testClaims{UserID: "u1"})
				if err != nil {
					t.Fatalf("GenerateAccess: %v", err)
				}
				parsed, err := svc.Parse(access)
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				if parsed.UserID != "u1" {
					t.Fatalf("UserID = %q, want u1", parsed.UserID)
				}
				refresh, err := svc.GenerateRefresh(&testClaims{UserID: "u1"})
				if err != nil {
					t.Fatalf("GenerateRefresh: %v", err)
				}
				if _, err := svc.ParseRefresh(refresh); err != nil {
					t.Fatalf("ParseRefresh: %v", err)
				}
			})
		}
	}
}

func TestParse_RejectsUnexpectedSigningMethod(t *testing.T) {
	t.Parallel()
	hmacSvc, err := NewService(newTestConfig(), func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	token, err := hmacSvc.GenerateAccess(&testClaims{UserID: "u1"})
	if err != nil {
		t.Fatalf("GenerateAccess: %v", err)
	}
	rsSvc, err := NewService(asymmetricConfig(t, RS256, false), func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService RS256: %v", err)
	}
	if _, err := rsSvc.Parse(token); err == nil {
		t.Fatal("expected rejection of HS256 token by RS256 verifier")
	}
}

func TestApplyDefaultsSetsMethod(t *testing.T) {
	t.Parallel()
	cfg := &Config{}
	cfg.ApplyDefaults()
	if cfg.Method != RS256 {
		t.Fatalf("default method = %q, want RS256", cfg.Method)
	}
}

func TestFindRegisteredClaims_TypedNilPointer(t *testing.T) {
	t.Parallel()
	var typedNil *testClaims
	if rc := findRegisteredClaims(typedNil); rc != nil {
		t.Fatal("expected nil for typed nil pointer")
	}
}

func TestGenerateReturnsSignError(t *testing.T) {
	t.Parallel()
	// A Service configured for RS256 but given a non-RSA sign key forces
	// SignedString to fail without going through NewService validation.
	svc := &Service[*testClaims]{cfg: Config{Method: RS256, PrivateKey: "not-a-key"}, newEmpty: func() *testClaims { return &testClaims{} }}
	if _, err := svc.Generate(&testClaims{UserID: "u1"}); err == nil {
		t.Fatal("expected sign error from Generate")
	}
	if _, err := svc.generateWithKey(&testClaims{UserID: "u1"}, "not-a-key"); err == nil {
		t.Fatal("expected sign error from generateWithKey")
	}
}

func TestValidateRequiredClaims(t *testing.T) {
	t.Parallel()
	svc, err := NewService(newTestConfig(), func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	now := time.Now()
	full := func() *testClaims {
		return &testClaims{RegisteredClaims: gojwt.RegisteredClaims{
			ExpiresAt: gojwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  gojwt.NewNumericDate(now),
			NotBefore: gojwt.NewNumericDate(now),
			Issuer:    "test-issuer",
			Audience:  gojwt.ClaimStrings{"test-audience"},
		}}
	}
	if err := svc.validateRequiredClaims(full()); err != nil {
		t.Fatalf("full claims rejected: %v", err)
	}

	mutators := map[string]func(*testClaims){
		"exp": func(c *testClaims) { c.ExpiresAt = nil },
		"iat": func(c *testClaims) { c.IssuedAt = nil },
		"nbf": func(c *testClaims) { c.NotBefore = nil },
		"iss": func(c *testClaims) { c.Issuer = "" },
		"aud": func(c *testClaims) { c.Audience = nil },
	}
	for name, mutate := range mutators {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			claims := full()
			mutate(claims)
			if err := svc.validateRequiredClaims(claims); err == nil {
				t.Fatalf("expected error for missing %s", name)
			}
		})
	}
}

func TestParse_RejectsTokenMissingNotBefore(t *testing.T) {
	t.Parallel()
	cfg := newTestConfig()
	svc, err := NewService(cfg, func() *testClaims { return &testClaims{} })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	now := time.Now()
	claims := &testClaims{RegisteredClaims: gojwt.RegisteredClaims{
		ExpiresAt: gojwt.NewNumericDate(now.Add(time.Hour)),
		IssuedAt:  gojwt.NewNumericDate(now),
		Issuer:    cfg.Issuer,
		Audience:  gojwt.ClaimStrings(cfg.Audience),
	}}
	token := gojwt.NewWithClaims(gojwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(cfg.Secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := svc.Parse(signed); err == nil {
		t.Fatal("expected rejection of token missing nbf claim")
	}
}

// FuzzParse exercises the JWT Service.Parse path with arbitrary input bytes.
// The contract is: Parse must not panic on any input. Invalid tokens must
// surface as errors, not crashes. Algorithm-confusion seeds (alg=none,
// alg=HS256-against-RSA-key, malformed compact form, oversize segments) are
// added to ensure the corpus exercises the security-critical paths.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		".",
		"..",
		"a.b.c",
		jwtSeed(`{"alg":"none","typ":"JWT"}`, `{"sub":"1234567890"}`, ""),        // alg=none
		jwtHeaderOnly(`{"alg":"HS256","typ":"JWT"}`),                             // truncated (header only)
		jwtSeed(`{"alg":"HS256","typ":"JWT"}`, `{"sub":"1234567890"}`, "badsig"), // bad signature
		jwtSeed(`{"alg":"RS256","typ":"JWT"}`, `{"sub":"1234567890"}`, "badsig"), // alg confusion (RS256 header, HMAC key)
		"\x00\x00\x00",
		string(make([]byte, 64*1024)),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	type fuzzClaims struct {
		gojwt.RegisteredClaims
	}
	cfg := &Config{
		Method:             HS256,
		Secret:             "fuzz-secret-32-bytes-or-more-for-test",
		AllowSymmetricHMAC: true,
		Issuer:             "fuzz-issuer",
		Audience:           []string{"fuzz-audience"},
	}
	svc, err := NewService(cfg, func() *fuzzClaims { return &fuzzClaims{} })
	if err != nil {
		f.Fatalf("NewService: %v", err)
	}

	f.Fuzz(func(t *testing.T, token string) {
		// Contract: never panic. Errors are expected on garbage input.
		_, _ = svc.Parse(token)
	})
}
