package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// BodySizeLimit restricts the request body to the given size string (e.g. "10MB").
func BodySizeLimit(maxSize string) gin.HandlerFunc {
	size := parseSize(maxSize)
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, size)
		c.Next()
	}
}

func parseSize(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))
	var multiplier int64 = 1
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		s = s[:len(s)-2]
	}
	var val int64
	if _, err := fmt.Sscanf(s, "%d", &val); err == nil {
		return val * multiplier
	}
	return 10 * 1024 * 1024 // default 10MB
}
