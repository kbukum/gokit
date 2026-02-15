package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/skillsenselab/gokit/logger"
)

// Recovery returns a Gin middleware that recovers from panics and logs the stack.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("Panic recovered", map[string]interface{}{
					"error":     fmt.Sprintf("%v", err),
					"stack":     string(debug.Stack()),
					"path":      c.Request.URL.Path,
					"method":    c.Request.Method,
					"client_ip": c.ClientIP(),
				})
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": "Internal server error",
				})
			}
		}()
		c.Next()
	}
}
