package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSConfig holds CORS middleware configuration.
type CORSConfig struct {
	AllowedOrigins   []string `yaml:"allowed_origins" mapstructure:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods" mapstructure:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers" mapstructure:"allowed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials" mapstructure:"allow_credentials"`
}

// CORS returns a Gin middleware configured from CORSConfig.
func CORS(cfg CORSConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if isAllowedOrigin(origin, cfg.AllowedOrigins) {
			c.Header("Access-Control-Allow-Origin", origin)
		}
		if len(cfg.AllowedMethods) > 0 {
			c.Header("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
		}
		if len(cfg.AllowedHeaders) > 0 {
			c.Header("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
		}
		if cfg.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func isAllowedOrigin(origin string, allowed []string) bool {
	for _, a := range allowed {
		if origin == a || a == "*" {
			return true
		}
	}
	return false
}
