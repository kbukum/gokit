package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/util"
)

const defaultMaxBodySize = 10 * 1024 * 1024 // 10MB

// BodySizeLimit returns middleware that restricts the request body to the given
// size string (e.g. "10MB", "512KB", "1GB").
func BodySizeLimit(maxSize string) Middleware {
	size := util.ParseSize(maxSize, defaultMaxBodySize)
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
