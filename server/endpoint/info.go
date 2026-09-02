package endpoint

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/version"
)

// startTime records when the process started for uptime calculation.
var startTime = time.Now()

// Info returns a handler that reports service version and build information.
func Info(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		v := version.GetVersionInfo()
		c.JSON(http.StatusOK, InfoResponse{
			Service:   serviceName,
			Version:   v.Version,
			GitCommit: v.GitCommit,
			GitBranch: v.GitBranch,
			BuildTime: v.BuildTime,
			GoVersion: v.GoVersion,
			IsRelease: v.IsRelease,
			IsDirty:   v.IsDirty,
			Uptime:    time.Since(startTime).String(),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
	}
}
