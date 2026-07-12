package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &CORSConfig{
		AllowedOrigins:   []string{"https://app.example"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"X-Custom"},
		AllowCredentials: true,
	}
	r := gin.New()
	r.Use(GinCORS(cfg))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Preflight
	req := httptest.NewRequest(http.MethodOptions, "/", http.NoBody)
	req.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("preflight: got %d want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Errorf("origin header missing: %v", w.Header())
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("credentials header missing")
	}

	// Disallowed origin → no CORS headers.
	req = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Origin", "https://evil")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("origin should be blocked")
	}

	// Wildcard.
	r2 := gin.New()
	r2.Use(GinCORS(&CORSConfig{AllowedOrigins: []string{"*"}}))
	r2.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	req = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Origin", "https://anywhere")
	w = httptest.NewRecorder()
	r2.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "https://anywhere" {
		t.Errorf("wildcard should allow any origin")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// requestid.go
// ─────────────────────────────────────────────────────────────────────────────
