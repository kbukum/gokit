// Package jwt provides a generic JWT token service using Go generics.
//
// The service is parameterized by a custom claims type T, which must implement
// jwt.Claims (typically by embedding jwt.RegisteredClaims). This allows each
// project to define its own claims structure without gokit knowing about it.
//
// Usage:
//
//	type MyClaims struct {
//	    jwt.RegisteredClaims
//	    UserID   string `json:"user_id"`
//	    TenantID string `json:"tenant_id"`
//	}
//
//	svc, err := jwt.NewService(cfg, func() *MyClaims { return &MyClaims{} })
//	token, err := svc.Generate(&MyClaims{
//	    RegisteredClaims: jwt.RegisteredClaims{Subject: "user-123"},
//	    UserID: "user-123",
//	})
//	claims, err := svc.Parse(tokenString)
package jwt

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"
)

// Service provides JWT token generation and parsing for custom claims type T.
// T must implement jwt.Claims (e.g., by embedding jwt.RegisteredClaims).
type Service[T gojwt.Claims] struct {
	cfg      Config
	newEmpty func() T
}

// NewService creates a new JWT service.
// The newEmpty function returns a zero-value instance of T for parsing.
//
// Example:
//
//	svc, err := jwt.NewService(cfg, func() *MyClaims { return &MyClaims{} })
func NewService[T gojwt.Claims](cfg *Config, newEmpty func() T) (*Service[T], error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("jwt: %w", err)
	}
	return &Service[T]{cfg: *cfg, newEmpty: newEmpty}, nil
}

// Generate creates a signed JWT token from the given claims.
// If RegisteredClaims.ExpiresAt is zero, it is set to now + AccessTokenTTL.
// If RegisteredClaims.IssuedAt is zero, it is set to now.
func (s *Service[T]) Generate(claims T) (string, error) {
	token := gojwt.NewWithClaims(s.cfg.signingMethod(), claims)
	signed, err := token.SignedString(s.cfg.signKey())
	if err != nil {
		return "", fmt.Errorf("jwt: sign token: %w", err)
	}
	return signed, nil
}

// GenerateAccess creates a signed access token with standard time claims.
// It calls prepareClaims with AccessTokenTTL before signing.
func (s *Service[T]) GenerateAccess(claims T) (string, error) {
	s.prepareClaims(claims, s.cfg.AccessTokenTTL)
	return s.Generate(claims)
}

// GenerateRefresh creates a signed refresh token with standard time claims.
// It calls prepareClaims with RefreshTokenTTL before signing.
func (s *Service[T]) GenerateRefresh(claims T) (string, error) {
	s.prepareClaims(claims, s.cfg.RefreshTokenTTL)
	return s.generateWithKey(claims, s.cfg.refreshSignKey())
}

// Parse validates and parses a JWT token string into claims of type T.
// It verifies the signature, expiry, issuer, and audience. The service
// configuration must include Issuer and Audience, and the token must contain
// matching iss and aud claims.
func (s *Service[T]) Parse(tokenString string) (T, error) {
	return s.parseWithKey(tokenString, s.cfg.verifyKey())
}

// ParseRefresh validates and parses a refresh token string into claims of type T
// using the configured signature, expiry, issuer, and audience checks.
func (s *Service[T]) ParseRefresh(tokenString string) (T, error) {
	return s.parseWithKey(tokenString, s.cfg.refreshVerifyKey())
}

func (s *Service[T]) generateWithKey(claims T, signKey any) (string, error) {
	token := gojwt.NewWithClaims(s.cfg.signingMethod(), claims)
	signed, err := token.SignedString(signKey)
	if err != nil {
		return "", fmt.Errorf("jwt: sign token: %w", err)
	}
	return signed, nil
}

func (s *Service[T]) parseWithKey(tokenString string, verifyKey any) (T, error) {
	claims := s.newEmpty()
	token, err := gojwt.ParseWithClaims(
		tokenString,
		claims,
		s.keyFunc(verifyKey),
		s.parserOptions()...,
	)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("jwt: parse token: %w", err)
	}
	if !token.Valid {
		var zero T
		return zero, errors.New("jwt: invalid token")
	}
	parsed, ok := token.Claims.(T)
	if !ok {
		var zero T
		return zero, errors.New("jwt: unexpected claims type")
	}
	if err := s.validateRequiredClaims(parsed); err != nil {
		var zero T
		return zero, fmt.Errorf("jwt: %w", err)
	}
	return parsed, nil
}

// ValidatorFunc returns a function that validates a token string and returns
// the parsed claims as any. This bridges the typed JWT service with generic
// middleware that doesn't know about the specific claims type.
//
// To get an auth.TokenValidator interface, wrap with auth.TokenValidatorFunc:
//
//	validator := auth.TokenValidatorFunc(svc.ValidatorFunc())
//
// Or use the convenience helper:
//
//	validator := auth.NewJWTValidator(svc)
func (s *Service[T]) ValidatorFunc() func(string) (any, error) {
	return func(token string) (any, error) {
		return s.Parse(token)
	}
}

// keyFunc is the jwt.Keyfunc used during token parsing.
func (s *Service[T]) keyFunc(verifyKey any) gojwt.Keyfunc {
	return func(token *gojwt.Token) (interface{}, error) {
		// Verify signing method matches expected
		expected := s.cfg.signingMethod()
		if token.Method.Alg() != expected.Alg() {
			return nil, fmt.Errorf("jwt: unexpected signing method: %s", token.Method.Alg())
		}
		return verifyKey, nil
	}
}

// parserOptions returns jwt.ParserOption based on config.
func (s *Service[T]) parserOptions() []gojwt.ParserOption {
	opts := []gojwt.ParserOption{
		gojwt.WithValidMethods([]string{s.cfg.signingMethod().Alg()}),
		gojwt.WithLeeway(s.cfg.ClockSkew),
		gojwt.WithExpirationRequired(),
		gojwt.WithIssuedAt(),
	}
	if s.cfg.Issuer != "" {
		opts = append(opts, gojwt.WithIssuer(s.cfg.Issuer))
	}
	if len(s.cfg.Audience) > 0 {
		for _, aud := range s.cfg.Audience {
			opts = append(opts, gojwt.WithAudience(aud))
		}
	}
	return opts
}

func (s *Service[T]) validateRequiredClaims(claims T) error {
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return errors.New("missing required exp claim")
	}
	iat, err := claims.GetIssuedAt()
	if err != nil || iat == nil {
		return errors.New("missing required iat claim")
	}
	nbf, err := claims.GetNotBefore()
	if err != nil || nbf == nil {
		return errors.New("missing required nbf claim")
	}
	iss, err := claims.GetIssuer()
	if err != nil || iss == "" {
		return errors.New("missing required iss claim")
	}
	aud, err := claims.GetAudience()
	if err != nil || len(aud) == 0 {
		return errors.New("missing required aud claim")
	}
	return nil
}

// prepareClaims sets standard RegisteredClaims fields if they're accessible.
// It supports three approaches (tried in order):
//  1. SetDefaults interface — cleanest, gives full control to the claims type
//  2. Reflection — finds embedded RegisteredClaims in any custom struct
//  3. Direct cast — only works if T is literally *jwt.RegisteredClaims
func (s *Service[T]) prepareClaims(claims T, ttl time.Duration) {
	now := time.Now()

	// Path 1: ClaimsWithDefaults interface (preferred)
	if setter, ok := any(claims).(interface {
		SetDefaults(time.Time, time.Duration, string, []string)
	}); ok {
		setter.SetDefaults(now, ttl, s.cfg.Issuer, s.cfg.Audience)
		return
	}

	// Path 2: Use reflection to find embedded RegisteredClaims field
	if rc := findRegisteredClaims(any(claims)); rc != nil {
		if rc.ExpiresAt == nil || rc.ExpiresAt.IsZero() {
			rc.ExpiresAt = gojwt.NewNumericDate(now.Add(ttl))
		}
		if rc.IssuedAt == nil {
			rc.IssuedAt = gojwt.NewNumericDate(now)
		}
		if rc.NotBefore == nil {
			rc.NotBefore = gojwt.NewNumericDate(now)
		}
		if rc.Issuer == "" && s.cfg.Issuer != "" {
			rc.Issuer = s.cfg.Issuer
		}
		if len(rc.Audience) == 0 && len(s.cfg.Audience) > 0 {
			rc.Audience = gojwt.ClaimStrings(s.cfg.Audience)
		}
	}
}

// findRegisteredClaims uses reflection to find an embedded
// jwt.RegisteredClaims field in a struct (possibly behind a pointer).
// Returns a pointer to the embedded field, or nil if not found.
func findRegisteredClaims(v any) *gojwt.RegisteredClaims {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil
	}

	// Look for a field of type jwt.RegisteredClaims
	rcType := reflect.TypeOf(gojwt.RegisteredClaims{})
	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		if field.Type() == rcType && field.CanAddr() {
			return field.Addr().Interface().(*gojwt.RegisteredClaims)
		}
	}
	return nil
}
