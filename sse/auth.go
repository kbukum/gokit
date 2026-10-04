package sse

import (
	"context"
	stderrors "errors"
	"net/http"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/util"
)

// Authenticator verifies a header or cookie credential before opening a stream and returns the application's typed identity.
type Authenticator[T any] interface {
	Authenticate(*http.Request) (T, error)
}

// AuthenticatorFunc adapts an authentication function.
type AuthenticatorFunc[T any] func(*http.Request) (T, error)

func (f AuthenticatorFunc[T]) Authenticate(r *http.Request) (T, error) {
	if f == nil {
		var zero T
		return zero, apperrors.Unauthorized("")
	}
	return f(r)
}

// TokenValidator is the context-aware structural bearer-token validation seam. The application's authorization resolver interprets its typed identity.
type TokenValidator[T any] interface {
	ValidateToken(context.Context, string) (T, error)
}

// BearerAuthenticator accepts exactly one authorization-header token. Missing, invalid, or query-only tokens fail closed. Validation diagnostics remain in the cause, never the public message.
func BearerAuthenticator[T any](v TokenValidator[T]) Authenticator[T] {
	return AuthenticatorFunc[T](func(r *http.Request) (T, error) {
		var zero T
		if util.IsNil(v) {
			return zero, WithChallenge(apperrors.Unauthorized(""), security.BearerAuthScheme)
		}
		token, present, err := security.ParseBearerHeader(r.Header)
		if err != nil || !present {
			return zero, WithChallenge(apperrors.Unauthorized("").WithCause(err), security.BearerAuthScheme)
		}
		claims, err := v.ValidateToken(r.Context(), token)
		if err != nil {
			return zero, WithChallenge(apperrors.Unauthorized("").WithCause(err), security.BearerAuthScheme)
		}
		if util.IsNil(claims) {
			return zero, WithChallenge(apperrors.Unauthorized(""), security.BearerAuthScheme)
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

type identityKey[T any] struct{}

func withIdentity[T any](ctx context.Context, identity T) context.Context {
	return context.WithValue(ctx, identityKey[T]{}, identity)
}

// IdentityFromContext returns the identity supplied to an Authenticated authorization resolver. Distinct identity types have distinct context keys.
func IdentityFromContext[T any](ctx context.Context) (T, bool) {
	v, ok := ctx.Value(identityKey[T]{}).(T)
	return v, ok && !util.IsNil(v)
}
