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
// and returns a 500 Problem Details error response.
func Recovery(log *logging.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { //nolint:contextcheck // panic recovery closure passes r.Context() to the panic logger explicitly
				if err := recover(); err != nil {
					logRecoveredPanic(r.Context(), log, err, r.URL.Path, r.Method, r.RemoteAddr)
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

// GinRecovery returns a Gin middleware that recovers from panics.
// Prefer using Recovery() at the server level via ApplyMiddleware() which covers all routes.
// Use this only when you need recovery on the Gin engine directly.
func GinRecovery() gin.HandlerFunc {
	return GinWrap(Recovery(nil))
}

// logRecoveredPanic logs a recovered panic with stack trace. If log is nil,
// the global logger is used.
func logRecoveredPanic(ctx context.Context, log *logging.Logger, err any, path, method, remoteAddr string) {
	fields := map[string]any{
		"error":     fmt.Sprintf("%v", err),
		"stack":     string(debug.Stack()),
		"path":      path,
		"method":    method,
		"remote_ip": remoteAddr,
	}
	if log != nil {
		log.ErrorCtx(ctx, "Panic recovered", fields)
	} else {
		logging.ErrorCtx(ctx, "Panic recovered", fields)
	}
}
