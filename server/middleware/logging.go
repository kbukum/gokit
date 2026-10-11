package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/logging"
)

// RequestLogger logs the registered route, method, status and duration using context correlation. Install inside Tracing to share its span and route. Raw URLs and header-supplied IDs are not logged. Health checks are skipped; a nil logger uses an instance-local default.
func RequestLogger(log *logging.Logger) Middleware {
	if log == nil {
		log = logging.NewDefault("middleware")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isHealthEndpoint(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			sw := newStatusWriter(w)
			next.ServeHTTP(sw, r)
			duration := time.Since(start)

			fields := map[string]any{
				"method":      traceMethod(r.Method),
				"path":        requestLogRoute(r),
				"status":      sw.status,
				"duration_ms": duration.Milliseconds(),
			}
			logByStatus(r.Context(), log, fields, sw.status)
		})
	}
}

// GinRequestLogger returns a Gin middleware for request logging using the injected logger.
// A nil logger falls back to a per-middleware default. Prefer using RequestLogger() at the
// server level via ApplyMiddleware() which covers all routes; use this only when you need
// logging on the Gin engine directly.
func GinRequestLogger(log *logging.Logger) gin.HandlerFunc {
	if log == nil {
		log = logging.NewDefault("middleware")
	}
	return func(c *gin.Context) {
		if isHealthEndpoint(c.Request.URL.Path) {
			c.Next()
			return
		}

		start := time.Now()
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		path := c.FullPath()
		if path == "" {
			path = "unmatched"
		}

		fields := map[string]any{
			"method":  traceMethod(c.Request.Method),
			"path":    path,
			"status":  status,
			"latency": latency.String(),
		}

		if status >= 500 {
			fields["size"] = c.Writer.Size()
		}
		if latency > 500*time.Millisecond {
			fields["slow"] = true
		}
		logByStatus(c.Request.Context(), log, fields, status)
	}
}

func isHealthEndpoint(path string) bool {
	healthPaths := []string{
		"/health", "/alive", "/ready", "/metrics",
		"/api/health", "/api/alive", "/api/ready", "/api/metrics",
	}
	for _, hp := range healthPaths {
		if path == hp {
			return true
		}
	}
	if len(path) > 4 && path[:4] == "/api" {
		for _, hp := range []string{"/health", "/alive", "/ready", "/metrics"} {
			if strings.HasSuffix(path, hp) {
				return true
			}
		}
	}
	return false
}

// logByStatus logs request fields at the appropriate level based on HTTP status code.
// The logger must be non-nil (constructors default it). Shared by both Gin and net/http
// request logger middleware.
func logByStatus(ctx context.Context, log *logging.Logger, fields map[string]any, status int) {
	switch {
	case status >= 500:
		log.ErrorCtx(ctx, "Request completed", fields)
	case status >= 400:
		log.WarnCtx(ctx, "Request completed", fields)
	default:
		log.DebugCtx(ctx, "Request completed", fields)
	}
}

func requestLogRoute(r *http.Request) string {
	if route, ok := r.Context().Value(traceRouteKey{}).(string); ok && route != "" {
		return route
	}
	if r.Pattern != "" {
		return r.Pattern
	}
	return "unmatched"
}
