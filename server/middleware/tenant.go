package middleware

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// TenantConfig configures the Tenant middleware.
type TenantConfig struct {
	// HeaderName is the HTTP header to read tenant ID from (default: "X-Tenant-ID").
	HeaderName string
	// Required, if true, returns 400 for requests without tenant ID.
	Required bool
	// Fallback is an optional default tenant ID used when the header is missing
	// and Required is false.
	Fallback string
}

// contextKey is an unexported type to prevent collisions with other packages.
type tenantContextKey struct{}

// tenantKey is the single key used to store tenant ID in context.
var tenantKey = tenantContextKey{}

// Tenant returns a Gin middleware that extracts tenant ID from request headers
// and stores it in the request context.
//
// The tenant ID is extracted from the configured header (default: "X-Tenant-ID").
// If not found and Required is true, returns 400 Bad Request.
// If not found and Fallback is set, uses the fallback tenant ID.
// Otherwise, proceeds with an empty tenant ID.
//
// Retrieve the tenant ID in handlers with:
//
//	tenantID, ok := middleware.TenantFromContext(c.Request.Context())
//	tenantID := middleware.MustTenantFromContext(c.Request.Context())
func Tenant(cfg TenantConfig) gin.HandlerFunc {
	if cfg.HeaderName == "" {
		cfg.HeaderName = "X-Tenant-ID"
	}

	return func(c *gin.Context) {
		tenantID := c.GetHeader(cfg.HeaderName)

		if tenantID == "" {
			if cfg.Required {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"error": "tenant ID required",
				})
				return
			}
			tenantID = cfg.Fallback
		}

		// Store tenant ID in request context
		ctx := context.WithValue(c.Request.Context(), tenantKey, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// TenantFromContext retrieves the tenant ID from the request context.
// Returns the tenant ID and true if found, or empty string and false otherwise.
func TenantFromContext(ctx context.Context) (string, bool) {
	val := ctx.Value(tenantKey)
	if val == nil {
		return "", false
	}
	tenantID, ok := val.(string)
	return tenantID, ok
}

// MustTenantFromContext retrieves the tenant ID from the request context.
// Panics if tenant ID is missing.
// Use in handlers where Tenant middleware guarantees tenant ID exists.
func MustTenantFromContext(ctx context.Context) string {
	tenantID, ok := TenantFromContext(ctx)
	if !ok {
		panic("tenant: ID not found in context")
	}
	return tenantID
}
