package sse

import (
	"context"
	stderrors "errors"
	"net/http"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// Authenticator verifies a header or cookie credential before opening a stream. Its identity is genuinely opaque and owned by the injected authentication provider, not by this transport.
type Authenticator interface {
	Authenticate(r *http.Request) (identity any, err error)
}

// AuthenticatorFunc adapts an authentication function.
type AuthenticatorFunc func(*http.Request) (any, error)

func (f AuthenticatorFunc) Authenticate(r *http.Request) (any, error) { return f(r) }

// TokenValidator is the structural bearer-token validation seam. Opaque claims are interpreted by the application's authorization resolver.
type TokenValidator interface {
	ValidateToken(token string) (any, error)
}

// BearerAuthenticator accepts exactly one authorization-header token. Missing, invalid, or query-only tokens fail closed. Validation diagnostics remain in the cause, never the public message.
func BearerAuthenticator(v TokenValidator) Authenticator {
	return AuthenticatorFunc(func(r *http.Request) (any, error) {
		if util.IsNil(v) {
			return nil, WithChallenge(apperrors.Unauthorized(""), security.BearerAuthScheme)
		}
		fields := strings.Fields(r.Header.Get("Authorization"))
		if len(fields) != 2 || !strings.EqualFold(fields[0], security.BearerAuthScheme) {
			return nil, WithChallenge(apperrors.Unauthorized(""), security.BearerAuthScheme)
		}
		claims, err := v.ValidateToken(fields[1])
		if err != nil {
			return nil, WithChallenge(apperrors.Unauthorized("").WithCause(err), security.BearerAuthScheme)
		}
		if util.IsNil(claims) {
			return nil, WithChallenge(apperrors.Unauthorized(""), security.BearerAuthScheme)
		}
		return claims, nil
	})
}

type challengeError struct {
	err       error
	challenge string
}

func (e *challengeError) Error() string         { return e.err.Error() }
func (e *challengeError) Unwrap() error         { return e.err }
func (e *challengeError) authChallenge() string { return e.challenge }

// WithChallenge adds a WWW-Authenticate challenge to an error. The endpoint emits it only on HTTP 401.
func WithChallenge(err error, challenge string) error {
	if util.IsNil(err) || challenge == "" {
		return err
	}
	return &challengeError{err: err, challenge: challenge}
}

func authChallengeFor(err error) string {
	if util.IsNil(err) {
		return ""
	}
	var c interface{ authChallenge() string }
	if stderrors.As(err, &c) {
		return c.authChallenge()
	}
	return ""
}

type identityKey struct{}

func withIdentity(ctx context.Context, identity any) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// IdentityFromContext returns the opaque identity supplied to an Authenticated authorization resolver.
func IdentityFromContext(ctx context.Context) (any, bool) {
	v := ctx.Value(identityKey{})
	return v, !util.IsNil(v)
}
