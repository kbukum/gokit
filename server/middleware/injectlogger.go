package middleware

import (
	"net/http"

	"github.com/kbukum/gokit/logging"
)

// InjectLogger returns middleware that stores log on each request's context, so
// downstream handlers and response helpers (e.g. RespondWithError) can retrieve
// it via logging.LoggerFromContext instead of reaching for a package global. A
// nil logger leaves the context untouched.
func InjectLogger(log *logging.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if log != nil {
				r = r.WithContext(logging.ContextWithLogger(r.Context(), log))
			}
			next.ServeHTTP(w, r)
		})
	}
}
