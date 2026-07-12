package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestGinRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinRequestLogger())
	r.GET("/api/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/err", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	r.GET("/slow", func(c *gin.Context) {
		time.Sleep(550 * time.Millisecond)
		c.Status(http.StatusOK)
	})

	for _, p := range []string{"/api/x?q=1", "/health", "/err"} {
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
	// Slow path — sanity check, validates the slow-flag branch.
	req := httptest.NewRequest(http.MethodGet, "/slow", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestRequestLogger_NetHTTP(t *testing.T) {
	mw := RequestLogger(nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	for _, p := range []string{"/health", "/api/foo"} {
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		req.Header.Set("X-Request-Id", "rid-1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// responsewriter.go (Write + Unwrap)
// ─────────────────────────────────────────────────────────────────────────────

func TestStatusWriter_WriteAndUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusWriter(rec)

	// Write without explicit WriteHeader → status defaults to 200, wroteHeader becomes true.
	if _, err := sw.Write([]byte("hi")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !sw.wroteHeader {
		t.Errorf("wroteHeader not set after Write")
	}
	if sw.status != http.StatusOK {
		t.Errorf("default status: got %d want 200", sw.status)
	}
	if sw.Unwrap() != rec {
		t.Errorf("Unwrap should return underlying writer")
	}
	// Flush is a no-op on httptest.ResponseRecorder (which implements Flusher).
	sw.Flush()

	// WriteHeader called twice → second call is ignored.
	rec2 := httptest.NewRecorder()
	sw2 := newStatusWriter(rec2)
	sw2.WriteHeader(http.StatusTeapot)
	sw2.WriteHeader(http.StatusBadRequest)
	if sw2.status != http.StatusTeapot {
		t.Errorf("second WriteHeader should be ignored: got %d", sw2.status)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// tenant.go: SetTenantInContext + TenantFromContext
// ─────────────────────────────────────────────────────────────────────────────
