package endpoint

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/component"
)

// Readiness returns a handler for K8s readiness probes.
// It checks component health via the HealthChecker to determine if the service can accept traffic.
func Readiness(serviceName string, checker HealthChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := "ready"
		httpStatus := http.StatusOK

		if checker != nil {
			for _, ch := range checker(c.Request.Context()) {
				if ch.Status == component.StatusUnhealthy {
					status = "not_ready"
					httpStatus = http.StatusServiceUnavailable
					break
				}
			}
		}

		c.JSON(httpStatus, ReadinessResponse{
			Status:    status,
			Service:   serviceName,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}
}
