package sse_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/sse"
)

type bearerValidator func(context.Context, string) (string, error)

func (v bearerValidator) ValidateToken(ctx context.Context, token string) (string, error) {
	return v(ctx, token)
}

type requestValueKey struct{}

func TestBearerBoundaryAndRequestContext(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		headers []string
		valid   bool
	}{
		{"valid", []string{"Bearer " + "test-token"}, true},
		{"duplicate", []string{"Bearer " + "test-token", "Bearer " + "test-token"}, false},
		{"oversized", []string{"Bearer " + strings.Repeat("a", 8193)}, false},
		{"empty", []string{""}, false},
		{"comma", []string{"Bearer " + "test-token,Bearer other"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.WithValue(t.Context(), requestValueKey{}, "trace"))
			cancel()
			calls := 0
			auth := sse.BearerAuthenticator(bearerValidator(func(got context.Context, token string) (string, error) {
				calls++
				if got != ctx || got.Err() != context.Canceled || token != "test-token" {
					t.Fatal("request context or token lost")
				}
				return "alice", nil
			}))
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/", http.NoBody)
			req.Header["Authorization"] = tc.headers
			_, err := auth.Authenticate(req)
			if (err == nil) != tc.valid || (calls == 1) != tc.valid {
				t.Fatalf("error=%v validator calls=%d", err, calls)
			}
		})
	}
}

func TestAuthenticatedTypedContextIsolationAndLifetime(t *testing.T) {
	t.Parallel()
	type otherIdentity string
	lifetime, revoke := context.WithCancel(t.Context())
	defer revoke()
	ctx := context.WithValue(t.Context(), requestValueKey{}, "trace")
	resolve := sse.Authenticated(sse.AuthenticatorFunc[string](func(*http.Request) (string, error) {
		return "alice", nil
	}), func(r *http.Request, identity string) (sse.Access, error) {
		got, ok := sse.IdentityFromContext[string](r.Context())
		if !ok || got != identity || r.Context().Value(requestValueKey{}) != "trace" {
			t.Fatal("typed identity or upstream value lost")
		}

		if _, found := sse.IdentityFromContext[otherIdentity](r.Context()); found {
			t.Fatal("identity leaked across types")
		}
		return sse.Access{Principal: got, Route: "private", Lifetime: lifetime}, nil
	})
	access, err := resolve(httptest.NewRequestWithContext(ctx, http.MethodGet, "/", http.NoBody))
	if err != nil || access.Lifetime != lifetime {
		t.Fatalf("lifetime lost: %v", err)
	}
	revoke()
	if access.Lifetime.Err() != context.Canceled {
		t.Fatal("revocation not propagated")
	}
}

func TestNilAuthenticatorFuncRejectsDirectCall(t *testing.T) {
	t.Parallel()
	var auth sse.AuthenticatorFunc[string]
	if _, err := auth.Authenticate(httptest.NewRequest(http.MethodGet, "/", http.NoBody)); err == nil {
		t.Fatal("nil function admitted request")
	}
}
