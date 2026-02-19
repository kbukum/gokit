package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/logger"
)

// RequestLogger returns middleware that logs every request with method,
// path, status code, and duration. Health-check paths are silently skipped.
func RequestLogger(log *logger.Logger) Middleware {
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

			fields := map[string]interface{}{
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

// GinRequestLogger returns a Gin middleware for request logging.
// Prefer using RequestLogger() at the server level via ApplyMiddleware() which
// covers all routes. Use this only when you need logging on the Gin engine directly.
func GinRequestLogger() gin.HandlerFunc {
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

		fields := map[string]interface{}{
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
		logByStatus(nil, fields, status)
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
// If log is nil, the global logger is used.
// Shared by both Gin and net/http request logger middleware.
func logByStatus(log *logger.Logger, fields map[string]interface{}, status int) {
	logErr := logger.Error
	logWarn := logger.Warn
	logDebug := logger.Debug
	if log != nil {
		logErr = log.Error
		logWarn = log.Warn
		logDebug = log.Debug
	}

	switch {
	case status >= 500:
		logErr("Request completed", fields)
	case status >= 400:
		logWarn("Request completed", fields)
	default:
		logDebug("Request completed", fields)
	}
}
