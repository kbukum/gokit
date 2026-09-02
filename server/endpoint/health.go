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
		status := string(component.StatusHealthy)
		var components []component.Health

		if checker != nil {
			components = checker(c.Request.Context())
			for _, ch := range components {
				if ch.Status == component.StatusUnhealthy {
					status = string(component.StatusUnhealthy)
					break
				}
				if ch.Status == component.StatusDegraded && status != string(component.StatusUnhealthy) {
					status = string(component.StatusDegraded)
				}
			}
		}

		httpStatus := http.StatusOK
		if status == string(component.StatusUnhealthy) {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, HealthResponse{
			Status:     status,
			Service:    serviceName,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Components: components,
		})
	}
}
