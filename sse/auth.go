package sse

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"reflect"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
)

// isNilValue reports whether v is nil or a nil-able dynamic value — a nil
// pointer, interface, map, slice, channel, or func — carried inside an otherwise
// non-nil interface. A typed-nil dependency such as (*Validator)(nil) or
// AuthenticatorFunc(nil) passes a plain v == nil guard yet panics when invoked,
// so the security seams normalize it to a fail-closed gate here rather than on
// the request path.
func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}

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

// failClosedAuthenticator rejects every connection with 401. It is installed
// when a nil [Authenticator] is supplied to [WithAuthenticator], turning a wiring
// mistake into a closed gate rather than a silently unauthenticated endpoint.
type failClosedAuthenticator struct{}

// Authenticate implements [Authenticator] by always rejecting.
func (failClosedAuthenticator) Authenticate(*http.Request) (any, error) {
	return nil, apperrors.Unauthorized("authenticator is not configured")
}

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
// access logs, proxies, and browser history. A missing header, a malformed
// bearer value (wrong scheme, whitespace-only or extra tokens), or a validation
// failure all reject the connection with 401 before the stream opens.
//
// A nil v is a wiring error: rather than panic on the request path, the returned
// authenticator fails closed and rejects every connection with 401, so a
// misconfigured endpoint can never authenticate a request.
func BearerAuthenticator(v TokenValidator) Authenticator {
	if isNilValue(v) {
		return AuthenticatorFunc(func(*http.Request) (any, error) {
			return nil, bearerReject(apperrors.Unauthorized("authenticator is not configured"))
		})
	}
	return AuthenticatorFunc(func(r *http.Request) (any, error) {
		header := r.Header.Get("Authorization")
		if header == "" {
			return nil, bearerReject(apperrors.Unauthorized("missing authorization header"))
		}
		// Parse exactly a scheme and a token; strings.Fields collapses runs of
		// whitespace and drops empties, so a whitespace-only credential and an
		// extra-token form are rejected here rather than handed to a permissive
		// validator.
		fields := strings.Fields(header)
		if len(fields) != 2 || !strings.EqualFold(fields[0], security.BearerAuthScheme) {
			return nil, bearerReject(apperrors.Unauthorized("invalid authorization scheme; expected 'Bearer <token>'"))
		}
		claims, err := v.ValidateToken(fields[1])
		if err != nil {
			// Preserve the validator's cause so callers can errors.Is/As it for
			// diagnostics; the response path canonicalizes and redacts it, so the
			// original message never reaches the client.
			return nil, bearerReject(apperrors.Unauthorized("invalid or expired token").WithCause(err))
		}
		return claims, nil
	})
}

// bearerChallenge is the WWW-Authenticate scheme a bearer rejection advertises.
var bearerChallenge = security.BearerAuthScheme

// challengeError carries a WWW-Authenticate challenge alongside a rejection so a
// scheme-specific gate (such as bearer) can emit a standards-compliant challenge
// without hard-coding one for every authenticator. It unwraps to the underlying
// [apperrors.AppError], so canonicalization and errors.Is/As still see it.
type challengeError struct {
	err       error
	challenge string
}

func (e *challengeError) Error() string         { return e.err.Error() }
func (e *challengeError) Unwrap() error         { return e.err }
func (e *challengeError) authChallenge() string { return e.challenge }

// WithChallenge tags a rejection with the WWW-Authenticate challenge it should
// advertise (for example [security.BearerAuthScheme]) so writeAuthError emits it
// on the resulting 401. A custom [Authenticator] returns
// WithChallenge(err, scheme) to give clients a standards-compliant challenge
// without every authenticator hard-coding one; the wrapped err still
// canonicalizes and unwraps for errors.Is/As. A challenge is only ever attached
// to a 401, never a 403. An empty challenge leaves the rejection unchanged.
func WithChallenge(err error, challenge string) error {
	if err == nil || challenge == "" {
		return err
	}
	return &challengeError{err: err, challenge: challenge}
}

// bearerReject tags a rejection as a bearer failure so writeAuthError emits a
// `WWW-Authenticate: Bearer` challenge on the 401.
func bearerReject(err error) error {
	return WithChallenge(err, bearerChallenge)
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

// writeAuthError rejects a connection before the stream opens, collapsing err
// into one of the two statuses the SSE auth contract promises and writing an RFC
// 9457 problem response. An [apperrors.Forbidden] error (an authenticated but
// unauthorized principal) becomes a canonical 403; every other rejection becomes
// a canonical 401. When the rejection carries a scheme challenge (a bearer gate),
// a `WWW-Authenticate` header is emitted on the 401 so bearer clients receive a
// standards-compliant challenge; it is never attached to a 403. The originating
// error's message is never surfaced, so an injected authenticator or resolver
// cannot leak credential or identity detail through the response body or an
// unexpected status. It returns the body-encoding error, if any, so the caller
// can log a failed write on the runtime HTTP path.
func writeAuthError(w http.ResponseWriter, err error) error {
	problem := canonicalAuthError(err).ToProblemDetail()
	if problem.Status == http.StatusUnauthorized {
		if challenge := authChallengeFor(err); challenge != "" {
			w.Header().Set("WWW-Authenticate", challenge)
		}
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(problem.Status)
	return json.NewEncoder(w).Encode(problem)
}

// authChallengeFor returns the WWW-Authenticate challenge a rejection advertises,
// or "" when it carries none.
func authChallengeFor(err error) string {
	// A typed-nil error (such as an injected (*AppError)(nil)) would panic when
	// errors.As unwraps it; it advertises no challenge, so short-circuit here.
	if isNilValue(err) {
		return ""
	}
	var c interface{ authChallenge() string }
	if stderrors.As(err, &c) {
		return c.authChallenge()
	}
	return ""
}

// canonicalAuthError maps any rejection onto a canonical [apperrors.Forbidden]
// (403) or [apperrors.Unauthorized] (401) with a generic message, discarding the
// original error's contents so nothing an injected implementation returns reaches
// the client.
func canonicalAuthError(err error) *apperrors.AppError {
	// A nil *AppError can be carried inside a non-nil error interface; AsAppError
	// then reports a match with a nil pointer, so guard it before reading Code to
	// avoid a nil dereference while handling the rejection.
	if appErr, ok := apperrors.AsAppError(err); ok && appErr != nil && appErr.Code == apperrors.ErrCodeForbidden {
		return apperrors.Forbidden("")
	}
	return apperrors.Unauthorized("")
}
