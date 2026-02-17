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
	return s.Generate(claims)
}

// Parse validates and parses a JWT token string into claims of type T.
// It verifies the signature, expiry, and optionally issuer/audience.
func (s *Service[T]) Parse(tokenString string) (T, error) {
	claims := s.newEmpty()
	token, err := gojwt.ParseWithClaims(tokenString, claims, s.keyFunc, s.parserOptions()...)
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
	return parsed, nil
}

// ValidatorFunc returns a function that validates a token string and returns
// the parsed claims as any. This bridges the typed JWT service with generic
// middleware that doesn't know about the specific claims type.
//
// Usage with middleware:
//
//	router.Use(middleware.Auth(svc.ValidatorFunc()))
func (s *Service[T]) ValidatorFunc() func(string) (any, error) {
	return func(token string) (any, error) {
		return s.Parse(token)
	}
}

// keyFunc is the jwt.Keyfunc used during token parsing.
func (s *Service[T]) keyFunc(token *gojwt.Token) (interface{}, error) {
	// Verify signing method matches expected
	expected := s.cfg.signingMethod()
	if token.Method.Alg() != expected.Alg() {
		return nil, fmt.Errorf("jwt: unexpected signing method: %s", token.Method.Alg())
	}
	return s.cfg.verifyKey(), nil
}

// parserOptions returns jwt.ParserOption based on config.
func (s *Service[T]) parserOptions() []gojwt.ParserOption {
	opts := []gojwt.ParserOption{
		gojwt.WithValidMethods([]string{s.cfg.signingMethod().Alg()}),
	}
	if s.cfg.Issuer != "" {
		opts = append(opts, gojwt.WithIssuer(s.cfg.Issuer))
	}
	if len(s.cfg.Audience) > 0 {
		opts = append(opts, gojwt.WithAudience(s.cfg.Audience[0]))
	}
	return opts
}

// prepareClaims sets standard RegisteredClaims fields if they're accessible.
// Works by attempting to extract RegisteredClaims via the jwt.Claims interface.
func (s *Service[T]) prepareClaims(claims T, ttl time.Duration) {
	now := time.Now()

	// Try to set via the ClaimsWithDefaults interface if the claims type supports it
	if setter, ok := any(claims).(interface {
		SetDefaults(time.Time, time.Duration, string, []string)
	}); ok {
		setter.SetDefaults(now, ttl, s.cfg.Issuer, s.cfg.Audience)
		return
	}
}
