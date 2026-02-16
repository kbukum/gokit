package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/logger"
)

// RequestLogger logs each HTTP request with method, path, status, and latency.
// Health-check endpoints are silently skipped.
func RequestLogger() gin.HandlerFunc {
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

		switch {
		case status >= 500:
			fields["size"] = c.Writer.Size()
			logger.Error("HTTP request failed", fields)
		case status >= 400:
			logger.Warn("HTTP client error", fields)
		default:
			if latency > 500*time.Millisecond {
				fields["slow"] = true
			}
			logger.Debug("HTTP request", fields)
		}
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
