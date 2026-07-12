package middleware

import (
	"net/http"

	"github.com/kbukum/gokit/security"
)

// SecurityHeaders returns middleware that applies secure-by-default response
// headers to every request: Content-Security-Policy, X-Content-Type-Options,
// Referrer-Policy, Permissions-Policy, X-Frame-Options, and
// Strict-Transport-Security.
//
// The header set is owned by the canonical security.HeadersConfig (L3); a nil
// cfg yields the secure defaults. Header values are resolved once at
// construction so the per-request cost is a fixed sequence of Header.Set calls.
// It returns an error when cfg is invalid.
func SecurityHeaders(cfg *security.HeadersConfig) (Middleware, error) {
	headers, err := cfg.HeaderMap()
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			for key, value := range headers {
				h.Set(key, value)
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}
