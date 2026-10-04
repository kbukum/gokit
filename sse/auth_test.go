package sse_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/security"
	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/sse/testutil"
)

func TestBearerAuthenticationHeaderAndChallenge(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, header, query string
		want                int
	}{
		{"valid", security.BearerAuthScheme + " token", "", 200},
		{"missing", "", "", 401},
		{"query ignored", "", "?token=token", 401},
		{"wrong scheme", "Basic token", "", 401},
		{"empty token", security.BearerAuthScheme + " ", "", 401},
		{"extra token", security.BearerAuthScheme + " one two", "", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := endpointBus(t)
			cfg := handlerConfig()
			cfg.Authorize = sse.Authenticated(sse.BearerAuthenticator(testutil.TokenValidator[string]{Claims: "alice"}),
				func(r *http.Request, identity string) (sse.Access, error) {
					seen, ok := sse.IdentityFromContext[string](r.Context())
					if !ok || seen != identity {
						return sse.Access{}, apperrors.Unauthorized("")
					}
					return sse.Access{Principal: "alice", Route: "alice"}, nil
				})
			h, err := sse.NewHandler(b, cfg)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/"+tc.query, http.NoBody)
			r.Header.Set("Authorization", tc.header)
			w := testutil.NewResponseWriter()
			// Successful authentication is followed by a deliberate transport failure to finish the handler.
			w.FlushErrorValue = errors.New("test flush")
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("%d: %s", w.Code, w.Body)
			}
			if tc.want == 401 && w.Header().Get("WWW-Authenticate") != security.BearerAuthScheme {
				t.Fatal("missing bearer challenge")
			}
			if tc.want == 401 && w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("authentication failure is cacheable")
			}
		})
	}
}

func TestBearerFailurePreservesCauseWithoutExposure(t *testing.T) {
	t.Parallel()
	cause := errors.New("credential diagnostic")
	r := httptest.NewRequest("GET", "/", http.NoBody)
	r.Header.Set("Authorization", security.BearerAuthScheme+" token")
	auth := sse.BearerAuthenticator(testutil.TokenValidator[string]{Err: cause})
	_, err := auth.Authenticate(r)
	if !errors.Is(err, cause) {
		t.Fatal("validator cause lost")
	}
	cfg := handlerConfig()
	cfg.Authorize = sse.Authenticated(auth, func(*http.Request, string) (sse.Access, error) {
		t.Fatal("unexpected resolver")
		return sse.Access{}, nil
	})
	h, createErr := sse.NewHandler(endpointBus(t), cfg)
	if createErr != nil {
		t.Fatal(createErr)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 || strings.Contains(w.Body.String(), cause.Error()) {
		t.Fatalf("unsafe failure: %s", w.Body)
	}
	if err.Error() == "" {
		t.Fatal("missing diagnostic error")
	}
}

func TestAuthenticationNilWiringAndIdentity(t *testing.T) {
	t.Parallel()
	var nilValidator *testutil.TokenValidator[*string]
	var nilAuthenticator sse.AuthenticatorFunc[*string]
	var nilClaims *string
	for _, auth := range []sse.Authenticator[*string]{
		sse.BearerAuthenticator[*string](nil), sse.BearerAuthenticator(nilValidator),
		sse.BearerAuthenticator(testutil.TokenValidator[*string]{Claims: nilClaims}),
		nilAuthenticator, testutil.AllowAuthenticator(nilClaims),
	} {
		cfg := handlerConfig()
		cfg.Authorize = sse.Authenticated(auth, func(*http.Request, *string) (sse.Access, error) {
			return sse.Access{Principal: "a", Route: "a"}, nil
		})
		h, err := sse.NewHandler(endpointBus(t), cfg)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest("GET", "/", http.NoBody)
		r.Header.Set("Authorization", security.BearerAuthScheme+" token")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("nil auth admitted: %d", w.Code)
		}
	}
	if _, ok := sse.IdentityFromContext[string](context.Background()); ok {
		t.Fatal("unexpected identity")
	}
}

func TestCustomAuthChallengeAndTypedNilFailure(t *testing.T) {
	t.Parallel()
	var missing *apperrors.AppError
	for _, tc := range []struct {
		err       error
		code      int
		challenge string
	}{
		{sse.WithChallenge(apperrors.Unauthorized(""), "Custom"), 401, "Custom"},
		{sse.WithChallenge(apperrors.Forbidden(""), "Custom"), 403, ""},
		{sse.WithChallenge(apperrors.Unauthorized(""), ""), 401, ""},
		{sse.WithChallenge(missing, "Custom"), 500, ""},
	} {
		cfg := handlerConfig()
		cfg.Authorize = func(*http.Request) (sse.Access, error) { return sse.Access{}, tc.err }
		h, err := sse.NewHandler(endpointBus(t), cfg)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", http.NoBody))
		if w.Code != tc.code || w.Header().Get("WWW-Authenticate") != tc.challenge {
			t.Fatalf("challenge/status: %d %v", w.Code, w.Header())
		}
	}
	if sse.WithChallenge(nil, "Custom") != nil {
		t.Fatal("nil changed")
	}
}
