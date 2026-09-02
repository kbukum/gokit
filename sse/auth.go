package sse

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
)

// Authenticator gates an SSE connection before the stream opens. It derives
// credentials from the request — header-only, never the query string — and
// returns an opaque identity on success or an error on rejection.
//
// The seam is injected by the composing application so the sse transport (L5)
// never imports the auth module (L6). Return an [apperrors.Forbidden] error to
// reject an authenticated-but-unauthorized principal with 403; any other error
// (including [apperrors.Unauthorized]) is treated as a missing/invalid
// credential and rejected with 401. See [BearerAuthenticator] for the common
// bearer-token case.
type Authenticator interface {
	// Authenticate verifies the request and returns opaque identity claims. The
	// identity type is genuinely caller-defined, so any is the documented
	// opaque-value exception here; consumers recover the concrete type via
	// [IdentityFromContext] or a [WithClientIdentity] resolver.
	Authenticate(r *http.Request) (identity any, err error)
}

// AuthenticatorFunc adapts an ordinary function to the [Authenticator] interface.
type AuthenticatorFunc func(r *http.Request) (any, error)

// Authenticate implements [Authenticator].
func (f AuthenticatorFunc) Authenticate(r *http.Request) (any, error) { return f(r) }

// TokenValidator validates a bearer token and returns opaque claims. It is
// declared locally so the transport layer (L5) never imports the auth module
// (L6): any concrete validator with this method — including auth.TokenValidator —
// satisfies it structurally and is injected by the composing application.
type TokenValidator interface {
	// ValidateToken parses and verifies token, returning opaque claims. The
	// claims type is genuinely caller-defined, so any is the documented
	// opaque-value exception here.
	ValidateToken(token string) (any, error)
}

// BearerAuthenticator returns an [Authenticator] that reads a bearer token from
// the Authorization header only and validates it with v.
//
// Credentials are never read from the URL query string: a token there leaks into
// access logs, proxies, and browser history. A missing header, a non-Bearer
// scheme, an empty token, or a validation failure all reject the connection with
// 401 before the stream opens.
func BearerAuthenticator(v TokenValidator) Authenticator {
	return AuthenticatorFunc(func(r *http.Request) (any, error) {
		header := r.Header.Get("Authorization")
		if header == "" {
			return nil, apperrors.Unauthorized("missing authorization header")
		}
		scheme, token, ok := strings.Cut(header, " ")
		if !ok || !strings.EqualFold(scheme, security.BearerAuthScheme) || token == "" {
			return nil, apperrors.Unauthorized("invalid authorization scheme; expected 'Bearer <token>'")
		}
		claims, err := v.ValidateToken(token)
		if err != nil {
			return nil, apperrors.Unauthorized("invalid or expired token")
		}
		return claims, nil
	})
}

// identityKey is the private context key under which a resolved identity is stored.
type identityKey struct{}

// withIdentity returns a child context carrying the authenticated identity.
func withIdentity(ctx context.Context, identity any) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

// IdentityFromContext returns the identity resolved by an [Authenticator] for the
// current SSE connection, or false when the connection was unauthenticated. The
// concrete type is caller-defined; callers type-assert to their own claims type.
func IdentityFromContext(ctx context.Context) (any, bool) {
	v := ctx.Value(identityKey{})
	return v, v != nil
}

// writeAuthError rejects a connection before the stream opens, mapping err to an
// RFC 9457 problem response. An [apperrors.AppError] carries its own HTTP status
// (403 for Forbidden, 401 for Unauthorized); any other error defaults to 401.
func writeAuthError(w http.ResponseWriter, err error) {
	appErr, ok := apperrors.AsAppError(err)
	if !ok {
		appErr = apperrors.Unauthorized(err.Error())
	}
	problem := appErr.ToProblemDetail()
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	_ = json.NewEncoder(w).Encode(problem)
}
