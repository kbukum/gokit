package middleware

import (
	"context"
	"errors"
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

// ErrNoTenantID is returned when tenant ID is not present in request context.
var ErrNoTenantID = errors.New("tenant: ID not found in context")

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

// MustTenantFromContext retrieves the tenant ID from context and panics if missing.
// For use in tests and application startup only; never call from HTTP handlers or middleware.
func MustTenantFromContext(ctx context.Context) string {
	tenantID, ok := TenantFromContext(ctx)
	if !ok {
		panic("tenant: ID not found in context")
	}
	return tenantID
}

// TenantFromContextOrError retrieves the tenant ID from context and returns an error when missing.
func TenantFromContextOrError(ctx context.Context) (string, error) {
	tenantID, ok := TenantFromContext(ctx)
	if !ok {
		return "", ErrNoTenantID
	}
	return tenantID, nil
}
