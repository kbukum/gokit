package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinBodySizeLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinBodySizeLimit("10MB"))
	r.POST("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("body within limit: got %d", w.Code)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// cors.go
// ─────────────────────────────────────────────────────────────────────────────
