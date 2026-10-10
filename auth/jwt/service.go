package jwt

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	gojwt "github.com/golang-jwt/jwt/v5"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

const maxTokenBytes = 8 << 10

// Service issues and verifies tokens of one profile. It is safe for concurrent use.
type Service[T Claims] struct {
	cfg       Config
	keys      keyRing
	clock     util.Clock
	newClaims func() T
	method    gojwt.SigningMethod
	parser    *gojwt.Parser
	wantType  string
}

// NewService validates the profile and key set. The clock drives issuance times, claim checks and key retirement;
// newClaims returns an empty claims value for each validation.
func NewService[T Claims](cfg Config, keys KeySet, clock util.Clock, newClaims func() T) (*Service[T], error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, apperrors.InvalidInput("jwt", err.Error()).WithCause(err)
	}
	if util.IsNil(clock) {
		return nil, apperrors.InvalidInput("jwt.clock", "clock is required")
	}
	if newClaims == nil {
		return nil, apperrors.InvalidInput("jwt.claims", "claims constructor is required")
	}
	ring, err := newKeyRing(cfg.Method, keys)
	if err != nil {
		return nil, apperrors.InvalidInput("jwt.keys", err.Error()).WithCause(err)
	}
	cfg.Audience = slices.Clone(cfg.Audience)
	method := cfg.signingMethod()
	return &Service[T]{
		cfg:       cfg,
		keys:      ring,
		clock:     clock,
		newClaims: newClaims,
		method:    method,
		wantType:  normalizeType(cfg.Type),
		parser:    gojwt.NewParser(append(claimRules(cfg), gojwt.WithValidMethods([]string{method.Alg()}), gojwt.WithTimeFunc(clock.Now), gojwt.WithStrictDecoding())...),
	}, nil
}

// claimRules are the profile's claim checks, shared by the parser and the expiry re-check.
func claimRules(cfg Config) []gojwt.ParserOption {
	return []gojwt.ParserOption{
		gojwt.WithLeeway(cfg.Leeway),
		gojwt.WithExpirationRequired(),
		gojwt.WithNotBeforeRequired(),
		gojwt.WithIssuedAt(),
		gojwt.WithIssuer(cfg.Issuer),
		gojwt.WithAudience(cfg.Audience...),
	}
}

// GenerateToken fills unset registered claims in place and signs the token. Issued-at defaults to now (whole seconds),
// not-before to issued-at, expiry to issued-at plus MaxLifetime, issuer to the profile issuer, and audience to the
// profile audience when exactly one is configured. Claims outside the profile are rejected.
func (s *Service[T]) GenerateToken(ctx context.Context, claims T) (string, error) {
	if failure := apperrors.FromContext(ctx, "jwt.generate"); failure != nil {
		return "", failure
	}
	if s.keys.signing == nil {
		return "", apperrors.InvalidInput("jwt.keys", "service is verify-only")
	}
	var zero T
	if claims == zero {
		return "", apperrors.InvalidInput("jwt.claims", "claims are required")
	}
	reg := claims.registered()
	if reg.IssuedAt == nil {
		reg.IssuedAt = gojwt.NewNumericDate(s.clock.Now().Truncate(time.Second))
	}
	if reg.NotBefore == nil {
		reg.NotBefore = reg.IssuedAt
	}
	if reg.ExpiresAt == nil {
		reg.ExpiresAt = gojwt.NewNumericDate(reg.IssuedAt.Add(s.cfg.MaxLifetime))
	}
	if reg.Issuer == "" {
		reg.Issuer = s.cfg.Issuer
	}
	if len(reg.Audience) == 0 && len(s.cfg.Audience) == 1 {
		reg.Audience = gojwt.ClaimStrings{s.cfg.Audience[0]}
	}
	if err := s.checkIssued(reg); err != nil {
		return "", apperrors.InvalidInput("jwt.claims", err.Error())
	}
	token := gojwt.NewWithClaims(s.method, claims)
	token.Header["kid"] = s.keys.signingID
	token.Header["typ"] = s.cfg.Type
	signed, err := token.SignedString(s.keys.signing)
	if err != nil {
		return "", apperrors.Internal(fmt.Errorf("sign token: %w", err))
	}
	if len(signed) > maxTokenBytes {
		return "", apperrors.InvalidInput("jwt.claims", fmt.Sprintf("token exceeds %d bytes", maxTokenBytes))
	}
	return signed, nil
}

func (s *Service[T]) checkIssued(reg *gojwt.RegisteredClaims) error {
	if reg.Issuer != s.cfg.Issuer {
		return errors.New("issuer does not match the profile")
	}
	if len(reg.Audience) == 0 {
		return errors.New("audience is required")
	}
	for _, aud := range reg.Audience {
		if !slices.Contains(s.cfg.Audience, aud) {
			return fmt.Errorf("audience %q is not in the profile", aud)
		}
	}
	return s.checkShape(reg)
}

// checkShape enforces the profile rules golang-jwt does not: time ordering, bounded lifetime and audience cardinality.
func (s *Service[T]) checkShape(reg *gojwt.RegisteredClaims) error {
	if reg.IssuedAt == nil || reg.NotBefore == nil || reg.ExpiresAt == nil {
		return errors.New("iat, nbf and exp are required")
	}
	if reg.NotBefore.Before(reg.IssuedAt.Time) || !reg.NotBefore.Before(reg.ExpiresAt.Time) {
		return errors.New("claims must satisfy iat <= nbf < exp")
	}
	if reg.ExpiresAt.Sub(reg.IssuedAt.Time) > s.cfg.MaxLifetime {
		return fmt.Errorf("lifetime exceeds %s", s.cfg.MaxLifetime)
	}
	if s.cfg.SingleAudience && len(reg.Audience) != 1 {
		return errors.New("exactly one audience is required")
	}
	return nil
}

// ValidateToken verifies the signature with the key named by "kid", the "typ" header, and every profile rule. An expired
// but otherwise authentic token returns ErrCodeTokenExpired; every other failure returns ErrCodeInvalidToken.
func (s *Service[T]) ValidateToken(ctx context.Context, tokenString string) (T, error) {
	var zero T
	if failure := apperrors.FromContext(ctx, "jwt.validate"); failure != nil {
		return zero, failure
	}
	if len(tokenString) > maxTokenBytes {
		return zero, apperrors.InvalidToken().WithCause(fmt.Errorf("token exceeds %d bytes", maxTokenBytes))
	}
	claims := s.newClaims()
	if claims == zero {
		return zero, apperrors.Internal(errors.New("claims constructor returned nil"))
	}
	if _, err := s.parser.ParseWithClaims(tokenString, claims, s.verificationKey); err != nil {
		if errors.Is(err, gojwt.ErrTokenExpired) && s.onlyExpired(claims) {
			return zero, apperrors.TokenExpired().WithCause(err)
		}
		return zero, apperrors.InvalidToken().WithCause(err)
	}
	if err := s.checkShape(claims.registered()); err != nil {
		return zero, apperrors.InvalidToken().WithCause(err)
	}
	return claims, nil
}

// onlyExpired reports whether an authentic, expired token would pass every other rule at the last instant it was
// valid, so expiry is its sole defect. The parser verified the signature before checking claims.
func (s *Service[T]) onlyExpired(claims T) bool {
	reg := claims.registered()
	if reg.ExpiresAt == nil || s.checkShape(reg) != nil {
		return false
	}
	lastValid := reg.ExpiresAt.Add(s.cfg.Leeway - time.Nanosecond)
	validator := gojwt.NewValidator(append(claimRules(s.cfg), gojwt.WithTimeFunc(func() time.Time { return lastValid }))...)
	return validator.Validate(claims) == nil
}

func (s *Service[T]) verificationKey(token *gojwt.Token) (any, error) {
	typ, ok := token.Header["typ"].(string)
	if !ok || normalizeType(typ) != s.wantType {
		return nil, errors.New("unexpected token type")
	}
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, errors.New("key id is required")
	}
	key, ok := s.keys.verify[kid]
	if !ok {
		return nil, errors.New("unknown key id")
	}
	if !key.retireAt.IsZero() && !s.clock.Now().Before(key.retireAt) {
		return nil, errors.New("key is retired")
	}
	return key.key, nil
}
