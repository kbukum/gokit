package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const defaultMaxBodySize int64 = 10 * 1024 * 1024 // 10MB

// BodySizeLimit returns middleware that restricts the request body to maxBytes.
// A non-positive maxBytes falls back to the 10MB default.
func BodySizeLimit(maxBytes int64) Middleware {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBodySize
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// GinBodySizeLimit returns a Gin middleware for body size limiting.
// Prefer using BodySizeLimit() at the server level via ApplyMiddleware() which covers all routes.
// Use this only when you need it on the Gin engine directly.
func GinBodySizeLimit(maxBytes int64) gin.HandlerFunc {
	return GinWrap(BodySizeLimit(maxBytes))
}
