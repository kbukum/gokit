package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TenantConfig configures the Tenant middleware.
type TenantConfig struct {
	HeaderName string
	Required   bool
	Fallback   string
}

type tenantContextKey struct{}

var tenantKey = tenantContextKey{}

// Tenant returns a Gin middleware that extracts tenant ID from request headers and stores it in context.
func Tenant(cfg TenantConfig) gin.HandlerFunc {
	if cfg.HeaderName == "" {
		cfg.HeaderName = "X-Tenant-ID"
	}

	return func(c *gin.Context) {
		tenantID := c.GetHeader(cfg.HeaderName)

		if tenantID == "" {
			if cfg.Required {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "tenant ID required"})
				return
			}
			tenantID = cfg.Fallback
		}

		ctx := context.WithValue(c.Request.Context(), tenantKey, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// TenantFromContext retrieves the tenant ID from the request context.
func TenantFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(tenantKey)
	if val == nil {
		return "", false
	}
	tenantID, ok := val.(string)
	return tenantID, ok
}

// SetTenantInContext stores a tenant ID in the context.
func SetTenantInContext(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantKey, tenantID)
}
