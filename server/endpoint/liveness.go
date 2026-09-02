package endpoint

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Liveness returns a handler for K8s liveness probes. It simply confirms the process is alive
// and able to serve HTTP.
func Liveness(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, LivenessResponse{
			Status:    "alive",
			Service:   serviceName,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}
}
