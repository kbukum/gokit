package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTenant_ExtractedFromHeader(t *testing.T) {
	cfg := TenantConfig{HeaderName: "X-Tenant-ID", Required: false}
	middleware := Tenant(cfg)

	// Create a test request with tenant header
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Tenant-ID", "tenant-123")
	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	middleware(c)

	if c.IsAborted() {
		t.Errorf("Expected middleware to not abort")
	}

	tenantID, ok := TenantFromContext(c.Request.Context())
	if !ok || tenantID != "tenant-123" {
		t.Errorf("Expected tenant-123, got %s (ok=%v)", tenantID, ok)
	}
}

func TestTenant_MissingWithRequired(t *testing.T) {
	cfg := TenantConfig{HeaderName: "X-Tenant-ID", Required: true}
	middleware := Tenant(cfg)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	middleware(c)

	if !c.IsAborted() {
		t.Errorf("Expected abort with 400")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestTenant_MissingWithFallback(t *testing.T) {
	cfg := TenantConfig{HeaderName: "X-Tenant-ID", Required: false, Fallback: "default-tenant"}
	middleware := Tenant(cfg)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	middleware(c)

	if c.IsAborted() {
		t.Errorf("Expected middleware to not abort")
	}

	tenantID, ok := TenantFromContext(c.Request.Context())
	if !ok || tenantID != "default-tenant" {
		t.Errorf("Expected default-tenant, got %s (ok=%v)", tenantID, ok)
	}
}

func TestTenant_MissingNotRequired(t *testing.T) {
	cfg := TenantConfig{HeaderName: "X-Tenant-ID", Required: false}
	middleware := Tenant(cfg)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	middleware(c)

	if c.IsAborted() {
		t.Errorf("Expected middleware to not abort")
	}

	tenantID, ok := TenantFromContext(c.Request.Context())
	if ok && tenantID != "" {
		t.Errorf("Expected empty tenant when not required and not provided, got %s", tenantID)
	}
}

func TestTenantFromContext_NotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	tenantID, ok := TenantFromContext(req.Context())

	if ok || tenantID != "" {
		t.Errorf("Expected not found, got %s (ok=%v)", tenantID, ok)
	}
}

func TestMustTenantFromContext_Panics(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)

	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic when tenant not found")
		}
	}()

	MustTenantFromContext(req.Context())
}

func TestMustTenantFromContext_Success(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	ctx := context.WithValue(req.Context(), tenantKey, "tenant-456")

	tenantID := MustTenantFromContext(ctx)
	if tenantID != "tenant-456" {
		t.Errorf("Expected tenant-456, got %s", tenantID)
	}
}

func TestTenant_DefaultHeaderName(t *testing.T) {
	cfg := TenantConfig{} // Empty HeaderName should default to "X-Tenant-ID"
	middleware := Tenant(cfg)

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Tenant-ID", "tenant-789")
	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	middleware(c)

	if c.IsAborted() {
		t.Errorf("Expected middleware to not abort")
	}

	tenantID, ok := TenantFromContext(c.Request.Context())
	if !ok || tenantID != "tenant-789" {
		t.Errorf("Expected tenant-789, got %s (ok=%v)", tenantID, ok)
	}
}
