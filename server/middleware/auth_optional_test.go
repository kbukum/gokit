package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeTokenValidator struct {
	claims any
	err    error
}

func (f fakeTokenValidator) ValidateToken(string) (any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

func TestOptionalAuth_RejectInvalidTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// Default behavior rejects invalid tokens (no WithAllowInvalidTokens needed).
	h, err := OptionalAuth(fakeTokenValidator{err: errors.New("invalid")})
	if err != nil {
		t.Fatalf("OptionalAuth() error: %v", err)
	}
	r.Use(h)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestOptionalAuth_AllowInvalidTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h, err := OptionalAuth(
		fakeTokenValidator{err: errors.New("invalid")},
		WithAllowInvalidTokens(true),
	)
	if err != nil {
		t.Fatalf("OptionalAuth() error: %v", err)
	}
	r.Use(h)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func BenchmarkMiddlewareStackExecution(b *testing.B) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	warn := func(_ *gin.Context, _ string) {}
	h, err := Auth(
		fakeTokenValidator{claims: map[string]string{"sub": "user"}},
		WithQueryTokenParam("token"),
		WithQueryTokenAllowedPaths("/bench"),
		WithQueryTokenWarningLogger(warn),
	)
	if err != nil {
		b.Fatalf("Auth() error: %v", err)
	}
	r.Use(h)
	r.GET("/bench", func(c *gin.Context) { c.Status(http.StatusOK) })

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/bench?token=abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("status = %d", w.Code)
		}
	}
}
