package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type contextualValidator func(context.Context, string) (string, error)

func (v contextualValidator) ValidateToken(ctx context.Context, token string) (string, error) {
	return v(ctx, token)
}

func TestAuthCredentialBoundary(t *testing.T) {
	t.Parallel()
	for _, optional := range []bool{false, true} {
		for _, tc := range []struct {
			name    string
			headers []string
			valid   bool
		}{
			{"missing", nil, false},
			{"empty", []string{""}, false},
			{"valid", []string{"Bearer " + "test-token"}, true},
			{"case insensitive", []string{"bEaReR " + "test-token"}, true},
			{"wrong scheme", []string{"Basic test-token"}, false},
			{"empty token", []string{"Bearer "}, false},
			{"duplicate", []string{"Bearer " + "test-token", "Bearer " + "test-token"}, false},
			{"comma joined", []string{"Bearer " + "test-token,Bearer other"}, false},
			{"extra field", []string{"Bearer " + "test-token other"}, false},
			{"oversized", []string{"Bearer " + strings.Repeat("a", 8193)}, false},
		} {
			t.Run(tc.name+map[bool]string{true: "/optional", false: "/required"}[optional], func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithCancel(context.WithValue(t.Context(), ctxClaimsKey{}, "upstream"))
				cancel()
				calls := 0
				validator := contextualValidator(func(got context.Context, token string) (string, error) {
					calls++
					if got.Err() != context.Canceled || got.Value(ctxClaimsKey{}) != "upstream" || token != "test-token" {
						t.Fatal("validator did not receive request context and token")
					}
					return "alice", nil
				})
				var handler gin.HandlerFunc
				var err error
				if optional {
					handler, err = OptionalAuth(validator, storeClaims)
				} else {
					handler, err = Auth(validator, storeClaims)
				}
				if err != nil {
					t.Fatal(err)
				}
				router := gin.New()
				router.Use(handler)
				router.GET("/", func(c *gin.Context) {
					if tc.valid && c.Request.Context().Value(ctxClaimsKey{}) != "alice" {
						t.Fatal("typed claims not propagated")
					}
					c.Status(http.StatusNoContent)
				})
				req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/?token=test-token", http.NoBody)
				if tc.headers != nil {
					req.Header["Authorization"] = tc.headers
				}
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, req)
				want := http.StatusUnauthorized
				if tc.valid || (optional && tc.headers == nil) {
					want = http.StatusNoContent
				}
				if recorder.Code != want || (calls == 1) != tc.valid {
					t.Fatalf("status=%d want=%d validator calls=%d", recorder.Code, want, calls)
				}
			})
		}
	}
}

func TestAuthRejectsInvalidIdentity(t *testing.T) {
	t.Parallel()
	for _, optional := range []bool{false, true} {
		r := gin.New()
		validator := fakeTokenValidator{err: errors.New("private credential diagnostic")}
		handler, err := Auth(validator, storeClaims)
		if optional {
			handler, err = OptionalAuth(validator, storeClaims)
		}
		if err != nil {
			t.Fatal(err)
		}
		r.Use(handler)
		r.GET("/", func(*gin.Context) { t.Fatal("invalid credential admitted") })
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+"test-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "private credential") {
			t.Fatalf("unsafe rejection: %d %s", w.Code, w.Body)
		}
	}
}

func TestAuthRejectsTypedNilDependencies(t *testing.T) {
	t.Parallel()
	var pointer *fakeTokenValidator
	var function contextualValidator
	for _, validator := range []TokenValidator[string]{nil, pointer, function} {
		if _, err := Auth(validator, storeClaims); err == nil {
			t.Fatal("accepted nil validator")
		}
		if _, err := OptionalAuth(validator, storeClaims); err == nil {
			t.Fatal("accepted nil validator")
		}
	}
	if _, err := Auth(fakeTokenValidator{}, nil); err == nil {
		t.Fatal("accepted nil setter")
	}
	if _, err := OptionalAuth(fakeTokenValidator{}, nil); err == nil {
		t.Fatal("accepted nil setter")
	}
	if _, err := Auth(fakeTokenValidator{}, storeClaims, nil); err == nil {
		t.Fatal("accepted nil option")
	}
}
