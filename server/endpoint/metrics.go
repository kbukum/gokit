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

		c.JSON(http.StatusOK, gin.H{
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"goroutines": runtime.NumGoroutine(),
			"memory": gin.H{
				"alloc_mb":       m.Alloc / 1024 / 1024,
				"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
				"sys_mb":         m.Sys / 1024 / 1024,
				"gc_runs":        m.NumGC,
			},
		})
	}
}
