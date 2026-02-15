package endpoint

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/skillsenselab/gokit/version"
)

// startTime records when the process started for uptime calculation.
var startTime = time.Now()

// Info returns a handler that reports service version and build information.
func Info(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		v := version.GetVersionInfo()
		c.JSON(http.StatusOK, gin.H{
			"service":    serviceName,
			"version":    v.Version,
			"git_commit": v.GitCommit,
			"git_branch": v.GitBranch,
			"build_time": v.BuildTime,
			"go_version": v.GoVersion,
			"is_release": v.IsRelease,
			"is_dirty":   v.IsDirty,
			"uptime":     time.Since(startTime).String(),
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
		})
	}
}
