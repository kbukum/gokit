package endpoint

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/version"
)

// Version returns a handler that reports build version information.
func Version() gin.HandlerFunc {
	return func(c *gin.Context) {
		v := version.GetVersionInfo()
		c.JSON(http.StatusOK, gin.H{
			"version":    v.Version,
			"git_commit": v.GitCommit,
			"git_branch": v.GitBranch,
			"build_time": v.BuildTime,
			"go_version": v.GoVersion,
			"is_release": v.IsRelease,
			"is_dirty":   v.IsDirty,
		})
	}
}
