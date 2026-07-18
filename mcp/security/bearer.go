package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

// ErrEmptyBearerToken indicates RequireBearerToken was configured with an empty token,
// which would leave the deployment unauthenticated.
var ErrEmptyBearerToken = errors.New("mcp: RequireBearerToken requires a non-empty token")

// RequireBearerToken wraps next with a middleware that requires a matching bearer token in the Authorization header.
// The token is never accepted from query strings.
// It fails closed on an empty token by returning ErrEmptyBearerToken
// so a misconfiguration cannot silently disable auth.
//
// Comparison of the presented and configured tokens is constant-time
// and does not leak the configured secret's length:
// both are reduced to a fixed-size SHA-256 digest before comparison.
// (Hashing the presented token is itself O(len(token)); the constant-time property covers the comparison, not the per-request hashing work.)
func RequireBearerToken(token string, next http.Handler) (http.Handler, error) {
	if token == "" {
		return nil, ErrEmptyBearerToken
	}
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := parseBearer(r.Header.Get("Authorization"))
		gotSum := sha256.Sum256([]byte(got))
		match := subtle.ConstantTimeCompare(gotSum[:], want[:]) == 1
		if !ok || !match {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	}), nil
}

// parseBearer extracts the token from an "Authorization: Bearer <token>" header value.
// The scheme match is case-insensitive per RFC 7235; the token itself is returned verbatim.
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
