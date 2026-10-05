package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
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
	o := &httpAuthOptions{writeError: WriteProblem}
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

// WriteProblem writes err as an application/problem+json response with Cache-Control: no-store. The error is
// normalized first, so causes and unknown errors never reach the client; a nil or invalid error becomes Internal.
func WriteProblem(w http.ResponseWriter, r *http.Request, err error) {
	w.Header().Set("Cache-Control", "no-store")
	failure := apperrors.Normalize(err)
	if failure == nil {
		failure = apperrors.Internal(nil)
	} else if validationErr := failure.Validate(); validationErr != nil {
		failure = apperrors.Internal(validationErr)
	}
	body, encodeErr := codec.Encode(codec.CompactJSON(), failure.ToProblemDetail())
	if encodeErr != nil {
		// Details may hold values JSON cannot encode; a bare Internal problem always encodes.
		failure = apperrors.Internal(encodeErr)
		if body, encodeErr = codec.Encode(codec.CompactJSON(), failure.ToProblemDetail()); encodeErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/problem+json")
	if failure.Retryable && failure.RetryAfter > 0 {
		seconds := failure.RetryAfter / time.Second
		if failure.RetryAfter%time.Second != 0 {
			seconds++
		}
		w.Header().Set("Retry-After", strconv.FormatInt(int64(seconds), 10))
	}
	w.WriteHeader(failure.HTTPStatus())
	if _, writeErr := w.Write([]byte(body)); writeErr != nil { //nolint:gosec // G705: the codec HTML-escapes JSON and the response is application/problem+json; covered by the encoding regression test.
		if log, ok := logging.LoggerFromContext(r.Context()); ok {
			log.WarnCtx(r.Context(), "HTTP authentication response write failed", map[string]any{"error": writeErr.Error()})
		}
	}
}
