package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

type requestAuthenticator func(*http.Request) (string, error)

// Authenticate reports every invocation as a presented credential; missingAuthenticator covers absence.
func (a requestAuthenticator) Authenticate(r *http.Request) (identity string, present bool, err error) {
	identity, err = a(r)
	return identity, true, err
}

func TestHTTPAuthPreservesRequestAndTypedClaims(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), ctxClaimsKey{}, "upstream"))
	cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/rpc", http.NoBody)
	req.Header["Cookie"] = []string{"session=one", "session=two"}
	auth := requestAuthenticator(func(got *http.Request) (string, error) {
		if got != req || got.Context().Err() != context.Canceled || got.Method != http.MethodPost || len(got.Cookies()) != 2 {
			t.Fatal("authentication lost raw request semantics")
		}
		return "alice", nil
	})
	middleware, err := HTTPAuth(auth, storeClaims)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(ctxClaimsKey{}) != "alice" || r.Context().Err() != context.Canceled {
			t.Fatal("typed principal or cancellation not propagated")
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestHTTPAuthNormalizesFailures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"invalid", apperrors.Unauthorized("").WithCause(errors.New("private credential")), 401},
		{"forbidden", apperrors.Forbidden(""), 403},
		{"store failure", errors.New("private credential store failure"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mw, err := HTTPAuth(requestAuthenticator(func(*http.Request) (string, error) {
				return "", tc.err
			}), storeClaims)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("failed authentication admitted")
			})).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", http.NoBody))
			if w.Code != tc.status || w.Header().Get("Content-Type") != "application/problem+json" || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "private credential") {
				t.Fatalf("response: %d %s", w.Code, w.Body)
			}
		})
	}
}

type nilIdentityAuthenticator struct{}

func (nilIdentityAuthenticator) Authenticate(*http.Request) (identity *string, present bool, err error) {
	return nil, true, nil // Exercise a provider that incorrectly reports success without an identity.
}

func TestHTTPAuthRejectsNilIdentity(t *testing.T) {
	t.Parallel()
	mw, err := HTTPAuth(nilIdentityAuthenticator{}, func(ctx context.Context, _ *string) context.Context {
		t.Fatal("nil identity reached setter")
		return ctx
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("nil identity reached handler")
	})).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("nil identity status=%d", w.Code)
	}
}

func TestHTTPAuthRetryHints(t *testing.T) {
	t.Parallel()
	for _, delay := range []time.Duration{time.Second, 1500 * time.Millisecond} {
		mw, err := HTTPAuth(requestAuthenticator(func(*http.Request) (string, error) {
			return "", apperrors.ServiceUnavailable("").WithRetryAfter(delay)
		}), storeClaims)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("unavailable authenticator admitted")
		})).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		want := "1"
		if delay > time.Second {
			want = "2"
		}
		if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != want {
			t.Fatalf("status=%d retry=%q", w.Code, w.Header().Get("Retry-After"))
		}
	}
}

func TestHTTPAuthNilDependencies(t *testing.T) {
	t.Parallel()
	var auth requestAuthenticator
	if _, err := HTTPAuth(auth, storeClaims); err == nil {
		t.Fatal("accepted typed nil authenticator")
	}
	if _, err := HTTPAuth[string](nil, storeClaims); err == nil {
		t.Fatal("accepted nil authenticator")
	}
	auth = func(*http.Request) (string, error) { return "alice", nil }
	if _, err := HTTPAuth(auth, nil); err == nil {
		t.Fatal("accepted nil setter")
	}
}

type missingAuthenticator struct{}

func (missingAuthenticator) Authenticate(*http.Request) (identity string, present bool, err error) {
	return "", false, nil
}

func TestHTTPAuthMissingPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		opts     []HTTPAuthOption
		wantCode int
	}{
		{name: "rejects by default", wantCode: http.StatusUnauthorized},
		{name: "rejects explicitly", opts: []HTTPAuthOption{WithMissingPolicy(RejectMissing)}, wantCode: http.StatusUnauthorized},
		{name: "accepts without identity", opts: []HTTPAuthOption{WithMissingPolicy(AcceptMissing)}, wantCode: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mw, err := HTTPAuth(missingAuthenticator{}, func(ctx context.Context, _ string) context.Context {
				t.Fatal("missing credentials stored an identity")
				return ctx
			}, tc.opts...)
			if err != nil {
				t.Fatal(err)
			}
			w := httptest.NewRecorder()
			mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if _, ok := r.Context().Value(ctxClaimsKey{}).(string); ok {
					t.Fatal("anonymous request carried an identity")
				}
				w.WriteHeader(http.StatusNoContent)
			})).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
			if w.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", w.Code, tc.wantCode)
			}
		})
	}
	if _, err := HTTPAuth(missingAuthenticator{}, storeClaims, WithMissingPolicy(MissingTokenPolicy(99))); err == nil {
		t.Fatal("accepted unknown missing-credential policy")
	}
}
