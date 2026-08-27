package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/logging"
)

// RequestLogger returns middleware that logs every request with method, path, status code,
// and duration. Health-check paths are silently skipped. A nil logger falls back to a
// per-middleware default so the constructor never reaches for a package global.
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
				"method":      r.Method,
				"path":        r.URL.Path,
				"status":      sw.status,
				"duration_ms": duration.Milliseconds(),
			}
			if id := r.Header.Get("X-Request-Id"); id != "" {
				fields["request_id"] = id
			}

			logByStatus(log, fields, sw.status)
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
		path := c.Request.URL.Path
		if q := c.Request.URL.RawQuery; q != "" {
			path = path + "?" + q
		}

		fields := map[string]any{
			"method":  c.Request.Method,
			"path":    path,
			"status":  status,
			"latency": latency.String(),
			"client":  c.ClientIP(),
		}

		if status >= 500 {
			fields["size"] = c.Writer.Size()
		}
		if latency > 500*time.Millisecond {
			fields["slow"] = true
		}
		logByStatus(log, fields, status)
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
func logByStatus(log *logging.Logger, fields map[string]any, status int) {
	switch {
	case status >= 500:
		log.Error("Request completed", fields)
	case status >= 400:
		log.Warn("Request completed", fields)
	default:
		log.Debug("Request completed", fields)
	}
}
