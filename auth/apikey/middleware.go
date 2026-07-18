package apikey

import (
	"context"
	"net/http"
)

// Validator is the interface used by the middleware to validate API keys.
// Implementations typically hash the key, look it up in the store, and check expiry.
type Validator interface {
	ValidateKey(ctx context.Context, plainKey string, requiredScopes ...string) (*Key, error)
}

// ValidatorFunc adapts a function to the Validator interface.
type ValidatorFunc func(ctx context.Context, plainKey string, requiredScopes ...string) (*Key, error)

// ValidateKey implements Validator.
func (f ValidatorFunc) ValidateKey(ctx context.Context, plainKey string, requiredScopes ...string) (*Key, error) {
	return f(ctx, plainKey, requiredScopes...)
}

// MiddlewareOption configures the API key middleware.
type MiddlewareOption func(*middlewareOptions)

type middlewareOptions struct {
	headerName string
	skipPaths  []string
}

// WithHeader sets the header name to read the API key from. Default: "X-API-Key".
func WithHeader(name string) MiddlewareOption {
	return func(o *middlewareOptions) { o.headerName = name }
}

// WithSkipPaths sets path prefixes to skip API key validation.
func WithSkipPaths(paths ...string) MiddlewareOption {
	return func(o *middlewareOptions) { o.skipPaths = paths }
}

// Middleware returns standard net/http middleware that validates API keys.
// If the configured header is absent,
// the request passes through (allowing other auth methods to handle it). If present but invalid,
// returns 401.
//
// On success, the validated Key is stored in the request context
// and can be retrieved with FromContext.
func Middleware(v Validator, opts ...MiddlewareOption) func(http.Handler) http.Handler {
	o := &middlewareOptions{headerName: "X-API-Key"}
	for _, opt := range opts {
		opt(o)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, prefix := range o.skipPaths {
				if len(r.URL.Path) >= len(prefix) && r.URL.Path[:len(prefix)] == prefix {
					next.ServeHTTP(w, r)
					return
				}
			}

			raw := r.Header.Get(o.headerName)
			if raw == "" {
				next.ServeHTTP(w, r)
				return
			}

			key, err := v.ValidateKey(r.Context(), raw)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid or expired API key"}`))
				return
			}

			ctx := context.WithValue(r.Context(), apikeyContextKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type contextKey struct{}

var apikeyContextKey = contextKey{}

// FromContext retrieves the validated API Key from the request context.
// Returns nil if no key was validated (e.g., request used a different auth method).
func FromContext(ctx context.Context) *Key {
	key, _ := ctx.Value(apikeyContextKey).(*Key)
	return key
}
