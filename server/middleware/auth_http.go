package middleware

import (
	"net/http"

	"github.com/kbukum/gokit/server/httpx"
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

// HTTPAuth authenticates before dispatch to HTTP, Connect, or SSE handlers. Apply it around the protected handler, not inside a unary interceptor: the original method, headers, cookies, and request context are authoritative. To let the same authentication decide trace trust, use [NewAuthStages] instead.
func HTTPAuth[T any](auth Authenticator[T], setClaims ClaimsSetter[T], opts ...HTTPAuthOption) (Middleware, error) {
	stages, err := NewAuthStages(auth, setClaims, opts...)
	if err != nil {
		return nil, err
	}
	return stages.Enforce, nil
}

// WriteProblemDetails adapts [httpx.WriteProblemDetails] to [AuthErrorWriter]. Causes and unknown errors never reach the client; nil, invalid and unencodable errors become Internal.
func WriteProblemDetails(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteProblemDetails(w, r, err)
}
