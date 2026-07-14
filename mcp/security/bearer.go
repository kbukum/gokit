package security

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireBearerToken wraps next with a middleware that requires a matching
// bearer token in the Authorization header, using a constant-time comparison.
// The token is never accepted from query strings. An empty token panics at
// construction time to prevent an accidentally unauthenticated deployment.
func RequireBearerToken(token string, next http.Handler) http.Handler {
	if token == "" {
		panic("mcp: RequireBearerToken requires a non-empty token")
	}
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := parseBearer(r.Header.Get("Authorization"))
		if !ok || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// parseBearer extracts the token from an "Authorization: Bearer <token>"
// header value. The scheme match is case-insensitive per RFC 7235; the token
// itself is returned verbatim.
func parseBearer(header string) (string, bool) {
	const prefix = "bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}
