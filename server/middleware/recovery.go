package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/logger"
)

// Recovery returns middleware that recovers from panics and returns a
// 500 JSON error response.
func Recovery(log *logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logRecoveredPanic(log, err, r.URL.Path, r.Method, r.RemoteAddr)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"Internal server error"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// GinRecovery returns a Gin middleware that recovers from panics.
// Prefer using Recovery() at the server level via ApplyMiddleware() which
// covers all routes. Use this only when you need recovery on the Gin engine directly.
func GinRecovery() gin.HandlerFunc {
	return GinWrap(Recovery(nil))
}

// logRecoveredPanic logs a recovered panic with stack trace.
// If log is nil, the global logger is used.
func logRecoveredPanic(log *logger.Logger, err interface{}, path, method, remoteAddr string) {
	fields := map[string]interface{}{
		"error":     fmt.Sprintf("%v", err),
		"stack":     string(debug.Stack()),
		"path":      path,
		"method":    method,
		"remote_ip": remoteAddr,
	}
	if log != nil {
		log.Error("Panic recovered", fields)
	} else {
		logger.Error("Panic recovered", fields)
	}
}
