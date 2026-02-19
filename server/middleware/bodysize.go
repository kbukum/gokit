package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// BodySizeLimit returns middleware that restricts the request body to the given
// size string (e.g. "10MB", "512KB", "1GB").
func BodySizeLimit(maxSize string) Middleware {
	size := parseSize(maxSize)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, size)
			next.ServeHTTP(w, r)
		})
	}
}

// GinBodySizeLimit returns a Gin middleware for body size limiting.
// Prefer using BodySizeLimit() at the server level via ApplyMiddleware() which
// covers all routes. Use this only when you need it on the Gin engine directly.
func GinBodySizeLimit(maxSize string) gin.HandlerFunc {
	return GinWrap(BodySizeLimit(maxSize))
}

func parseSize(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = s[:len(s)-2]
	}
	var val int64
	if _, err := fmt.Sscanf(s, "%d", &val); err == nil {
		return val * multiplier
	}
	return 10 * 1024 * 1024 // default 10MB
}
