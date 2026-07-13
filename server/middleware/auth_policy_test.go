package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMissingTokenPolicy_DefaultRejectsMissing(t *testing.T) {
	var p MissingTokenPolicy
	if p != RejectMissing {
		t.Fatalf("zero-value MissingTokenPolicy = %v, want RejectMissing", p)
	}
}

func TestMissingTokenPolicy_String(t *testing.T) {
	for p, want := range map[MissingTokenPolicy]string{
		RejectMissing:         "RejectMissing",
		AcceptMissing:         "AcceptMissing",
		MissingTokenPolicy(9): "RejectMissing",
	} {
		if got := p.String(); got != want {
			t.Errorf("MissingTokenPolicy(%d).String() = %q, want %q", p, got, want)
		}
	}
}

// TestMissingTokenPolicy_RejectInvalidAlways locks in the invariant that a
// present-but-invalid token is rejected under BOTH policies; the policy only
// governs *missing* credentials, never invalid ones.
func TestMissingTokenPolicy_RejectInvalidAlways(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name  string
		build func() (gin.HandlerFunc, error)
	}{
		{"reject-missing", func() (gin.HandlerFunc, error) {
			return Auth(fakeTokenValidator{err: errors.New("invalid")}, storeClaims)
		}},
		{"accept-missing", func() (gin.HandlerFunc, error) {
			return OptionalAuth(fakeTokenValidator{err: errors.New("invalid")}, storeClaims)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := tc.build()
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			r := gin.New()
			r.Use(h)
			r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req.Header.Set("Authorization", "Bearer bad")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
			}
		})
	}
}
