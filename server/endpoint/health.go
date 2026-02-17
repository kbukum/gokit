package endpoint

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/component"
)

// HealthChecker returns health status for registered components.
type HealthChecker func(ctx context.Context) []component.Health

// Health returns a handler that reports service health including component statuses.
func Health(serviceName string, checker HealthChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := "healthy"
		var components []component.Health

		if checker != nil {
			components = checker(c.Request.Context())
			for _, ch := range components {
				if ch.Status == component.StatusUnhealthy {
					status = "unhealthy"
					break
				}
				if ch.Status == component.StatusDegraded && status != "unhealthy" {
					status = "degraded"
				}
			}
		}

		httpStatus := http.StatusOK
		if status == "unhealthy" {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":     status,
			"service":    serviceName,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"components": components,
		})
	}
}
