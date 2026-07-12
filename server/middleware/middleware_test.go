package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinWrap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-MW", "1")
			next.ServeHTTP(w, req)
		})
	}
	r.Use(GinWrap(mw))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-MW") != "1" {
		t.Errorf("middleware not applied")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// bodysize.go
// ─────────────────────────────────────────────────────────────────────────────
