package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Middleware wraps an http.Handler with additional behavior.
// This is the standard Go middleware signature and the single middleware type
// for the entire server — it works with all routes including REST (Gin),
// ConnectRPC, and any other http.Handler mounted on the ServeMux.
type Middleware func(http.Handler) http.Handler

// Chain composes multiple middleware. The first in the list is the outermost
// (runs first on a request, last on a response).
func Chain(middlewares ...Middleware) Middleware {
	return func(final http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			final = middlewares[i](final)
		}
		return final
	}
}

// GinWrap adapts a standard Middleware for use in a Gin middleware chain.
// Use this when you need to apply a Middleware directly on the Gin engine
// instead of at the server handler level.
//
// Note: middleware that wraps http.ResponseWriter (e.g. RequestLogger) may not
// fully integrate with gin.Context.Writer. Prefer applying such middleware at
// the server level via ApplyMiddleware().
func GinWrap(mw Middleware) gin.HandlerFunc {
	return func(c *gin.Context) {
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			// Propagate any request modifications (e.g. added headers) back to Gin.
			c.Request = r
			c.Next()
		})
		mw(next).ServeHTTP(c.Writer, c.Request)
	}
}
