package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthConfig configures the JWT authentication middleware.
type AuthConfig struct {
	// TokenValidator validates a token string and returns the claims.
	TokenValidator func(token string) (map[string]interface{}, error)
	// SkipPaths are URL path prefixes that bypass authentication.
	SkipPaths []string
}

// Auth returns a Gin middleware that validates Bearer tokens using the
// configured TokenValidator. Validated claims are stored in the Gin context.
func Auth(cfg AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		for _, skip := range cfg.SkipPaths {
			if strings.HasPrefix(path, skip) {
				c.Next()
				return
			}
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid authorization header format",
			})
			return
		}

		claims, err := cfg.TokenValidator(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token",
			})
			return
		}

		for key, value := range claims {
			c.Set(key, value)
		}
		c.Next()
	}
}
