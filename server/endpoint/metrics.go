package endpoint

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// Metrics returns a handler that reports runtime memory and goroutine metrics.
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		c.JSON(http.StatusOK, MetricsResponse{
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			Goroutines: runtime.NumGoroutine(),
			Memory: MemoryMetrics{
				AllocMB:      m.Alloc / 1024 / 1024,
				TotalAllocMB: m.TotalAlloc / 1024 / 1024,
				SysMB:        m.Sys / 1024 / 1024,
				GCRuns:       m.NumGC,
			},
		})
	}
}
