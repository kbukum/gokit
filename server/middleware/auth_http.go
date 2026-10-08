package middleware

import (
	"net/http"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/server/httpx"
	"github.com/kbukum/gokit/util"
)

// Authenticator verifies the complete HTTP request and returns a typed identity. Implementations own credential precedence, cookie duplication, CSRF, expiry, and revocation checks. present is false only when the request genuinely carries no credential; any presented but malformed, ambiguous, or invalid credential must return an error.
type Authenticator[T any] interface {
	Authenticate(*http.Request) (identity T, present bool, err error)
}

// AuthErrorWriter encodes a rejection for the request's transport protocol. It receives the original authentication error, including private causes, and must normalize it before public serialization.
type AuthErrorWriter func(http.ResponseWriter, *http.Request, error)

// HTTPAuthOption configures complete-request authentication.
type HTTPAuthOption func(*httpAuthOptions)

type httpAuthOptions struct {
	writeError AuthErrorWriter
	missing    MissingTokenPolicy
}

// WithMissingPolicy sets how requests without any credential are treated. The default RejectMissing fails them; AcceptMissing dispatches them without storing an identity, so downstream guards still see an anonymous request. Invalid presented credentials are always rejected.
func WithMissingPolicy(policy MissingTokenPolicy) HTTPAuthOption {
	return func(o *httpAuthOptions) { o.missing = policy }
}

// WithAuthErrorWriter replaces the default RFC 9457 writer. Use a protocol-aware writer for a shared HTTP boundary that dispatches to Connect or other RPC transports.
func WithAuthErrorWriter(writer AuthErrorWriter) HTTPAuthOption {
	return func(o *httpAuthOptions) { o.writeError = writer }
}

// HTTPAuth authenticates before dispatch to HTTP, Connect, or SSE handlers. Apply it around the protected handler, not inside a unary interceptor: the original method, headers, cookies, and request context are authoritative.
func HTTPAuth[T any](auth Authenticator[T], setClaims ClaimsSetter[T], opts ...HTTPAuthOption) (Middleware, error) {
	if util.IsNil(auth) {
		return nil, apperrors.InvalidInput("authenticator", "a non-nil Authenticator is required")
	}
	if setClaims == nil {
		return nil, apperrors.InvalidInput("setClaims", "a non-nil ClaimsSetter is required")
	}
	o := &httpAuthOptions{writeError: WriteProblemDetails}
	for _, option := range opts {
		if option == nil {
			return nil, apperrors.InvalidInput("options", "authentication options must not be nil")
		}
		option(o)
	}
	if o.writeError == nil {
		return nil, apperrors.InvalidInput("writeError", "a non-nil AuthErrorWriter is required")
	}
	if o.missing != RejectMissing && o.missing != AcceptMissing {
		return nil, apperrors.InvalidInput("missing", "unknown missing-credential policy")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, present, err := auth.Authenticate(r)
			if err == nil && !present && o.missing == AcceptMissing {
				next.ServeHTTP(w, r)
				return
			}
			if err == nil && (!present || util.IsNil(identity)) {
				err = apperrors.Unauthorized("")
			}
			if err != nil {
				w.Header().Set("Cache-Control", "no-store")
				o.writeError(w, r, err)
				return
			}
			next.ServeHTTP(w, r.WithContext(setClaims(r.Context(), identity)))
		})
	}, nil
}

// WriteProblemDetails adapts [httpx.WriteProblemDetails] to [AuthErrorWriter]. Causes and unknown errors never reach the client; nil, invalid and unencodable errors become Internal.
func WriteProblemDetails(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteProblemDetails(w, r, err)
}
