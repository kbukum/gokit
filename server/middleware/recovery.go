package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

// Recovery returns middleware that recovers from panics
// and returns a 500 Problem Details error response. A nil logger falls back to a
// per-middleware default so the constructor never reaches for a package global.
func Recovery(log *logging.Logger) Middleware {
	if log == nil {
		log = logging.NewDefault("middleware")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { //nolint:contextcheck // panic recovery closure passes r.Context() to the panic logger explicitly
				if err := recover(); err != nil {
					logRecoveredPanic(r.Context(), log, err, r)
					pd := apperrors.Internal(fmt.Errorf("%v", err)).ToProblemDetail()
					pd.Instance = r.URL.Path
					w.Header().Set("Content-Type", "application/problem+json")
					w.WriteHeader(http.StatusInternalServerError)
					body, _ := json.Marshal(pd)
					_, _ = w.Write(body)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// GinRecovery returns a Gin middleware that recovers from panics using the injected logger.
// Prefer using Recovery() at the server level via ApplyMiddleware() which covers all routes.
// Use this only when you need recovery on the Gin engine directly.
func GinRecovery(log *logging.Logger) gin.HandlerFunc {
	return GinWrap(Recovery(log))
}

// logRecoveredPanic logs a recovered panic with stack trace. The logger must be
// non-nil (Recovery defaults it).
func logRecoveredPanic(ctx context.Context, log *logging.Logger, err any, r *http.Request) {
	fields := map[string]any{
		"error":     fmt.Sprintf("%v", err),
		"stack":     string(debug.Stack()),
		"path":      r.URL.Path,
		"method":    r.Method,
		"remote_ip": r.RemoteAddr,
	}
	log.ErrorCtx(ctx, "Panic recovered", fields)
}
