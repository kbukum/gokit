package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthSkipPath(t *testing.T) {
	t.Parallel()
	r := gin.New()
	h, err := Auth(fakeTokenValidator{}, storeClaims, WithSkipPaths("/public"))
	if err != nil {
		t.Fatal(err)
	}
	r.Use(h)
	r.GET("/public/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/public/x", http.NoBody))
	if w.Code != http.StatusOK {
		t.Fatalf("skip path: %d", w.Code)
	}
}

func TestRequire(t *testing.T) {
	t.Parallel()
	for _, allow := range []bool{true, false} {
		r := gin.New()
		r.Use(Require(func(*gin.Context) bool { return allow }))
		r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		want := http.StatusOK
		if !allow {
			want = http.StatusForbidden
		}
		if w.Code != want {
			t.Fatalf("allow=%v: status=%d want=%d", allow, w.Code, want)
		}
	}
}

type fakeChecker struct{ allow bool }

func (f fakeChecker) HasPermission(string, string) bool { return f.allow }

func TestRequirePermission(t *testing.T) {
	t.Parallel()
	for _, allow := range []bool{true, false} {
		r := gin.New()
		r.Use(RequirePermission(fakeChecker{allow: allow}, "read", func(*gin.Context) string { return "u1" }))
		r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		want := http.StatusOK
		if !allow {
			want = http.StatusForbidden
		}
		if w.Code != want {
			t.Fatalf("allow=%v: status=%d want=%d", allow, w.Code, want)
		}
	}
}
