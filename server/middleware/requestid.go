package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID returns middleware that ensures every request has an X-Request-Id header.
// If the client sends one it is preserved; otherwise a new UUID is generated.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ensureRequestID(r)
			w.Header().Set("X-Request-Id", id)
			next.ServeHTTP(w, r)
		})
	}
}

// GinRequestID returns a Gin middleware for request-ID injection.
// Prefer using RequestID() at the server level via ApplyMiddleware() which
// covers all routes. Use this only when you need it on the Gin engine directly.
func GinRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := ensureRequestID(c.Request)
		c.Set("request_id", id)
		c.Writer.Header().Set("X-Request-Id", id)
		c.Next()
	}
}

// ensureRequestID returns the existing X-Request-Id from the request or
// generates a new UUID and sets it on the request header.
func ensureRequestID(r *http.Request) string {
	id := r.Header.Get("X-Request-Id")
	if id == "" {
		id = uuid.New().String()
		r.Header.Set("X-Request-Id", id)
	}
	return id
}
