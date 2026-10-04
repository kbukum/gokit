package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeTokenValidator struct {
	claims string
	err    error
}

func (f fakeTokenValidator) ValidateToken(context.Context, string) (string, error) {
	return f.claims, f.err
}

type ctxClaimsKey struct{}

func storeClaims(ctx context.Context, claims string) context.Context {
	return context.WithValue(ctx, ctxClaimsKey{}, claims)
}

func BenchmarkMiddlewareStackExecution(b *testing.B) {
	b.ReportAllocs()
	r := gin.New()
	h, err := Auth(fakeTokenValidator{claims: "user"}, storeClaims)
	if err != nil {
		b.Fatal(err)
	}
	r.Use(h)
	r.GET("/bench", func(c *gin.Context) { c.Status(http.StatusOK) })
	b.ResetTimer()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/bench", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+"test-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("status = %d", w.Code)
		}
	}
}
