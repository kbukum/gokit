package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinRequestID())
	r.GET("/", func(c *gin.Context) {
		v, _ := c.Get("request_id")
		if v == nil || v.(string) == "" {
			t.Errorf("request_id not set in context")
		}
		c.Status(http.StatusOK)
	})

	// Auto-generated.
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") == "" {
		t.Errorf("X-Request-Id header missing")
	}

	// Preserved when client provides one.
	req = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("X-Request-Id", "given")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") != "given" {
		t.Errorf("client-provided request id not preserved")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// recovery.go: GinRecovery
// ─────────────────────────────────────────────────────────────────────────────
