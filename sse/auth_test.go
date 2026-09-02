package sse

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

type stubValidator struct {
	claims any
	err    error
}

func (s stubValidator) ValidateToken(string) (any, error) { return s.claims, s.err }

func TestBearerAuthenticator_HeaderOnly(t *testing.T) {
	t.Parallel()

	auth := BearerAuthenticator(stubValidator{claims: "user-1"})

	tests := []struct {
		name       string
		header     string
		query      string
		wantStatus int // 0 => success
	}{
		{name: "valid bearer", header: "Bearer good-token"},
		{name: "missing header", wantStatus: http.StatusUnauthorized},
		{name: "wrong scheme", header: "Basic abc", wantStatus: http.StatusUnauthorized},
		{name: "empty token", header: "Bearer ", wantStatus: http.StatusUnauthorized},
		{name: "whitespace-only token", header: "Bearer    ", wantStatus: http.StatusUnauthorized},
		{name: "extra token", header: "Bearer a b", wantStatus: http.StatusUnauthorized},
		{name: "token in query ignored", query: "token=good-token", wantStatus: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequest(http.MethodGet, "/events?"+tc.query, http.NoBody)
			if tc.header != "" {
				r.Header.Set("Authorization", tc.header)
			}
			identity, err := auth.Authenticate(r)
			if tc.wantStatus == 0 {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				if identity != "user-1" {
					t.Fatalf("expected identity user-1, got %v", identity)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected rejection, got success")
			}
			appErr, ok := apperrors.AsAppError(err)
			if !ok || appErr.HTTPStatus != tc.wantStatus {
				t.Fatalf("expected status %d, got %v", tc.wantStatus, err)
			}
		})
	}
}

func TestBearerAuthenticator_InvalidToken(t *testing.T) {
	t.Parallel()

	auth := BearerAuthenticator(stubValidator{err: errors.New("expired")})
	r := httptest.NewRequest(http.MethodGet, "/events", http.NoBody)
	r.Header.Set("Authorization", "Bearer whatever")

	_, err := auth.Authenticate(r)
	appErr, ok := apperrors.AsAppError(err)
	if !ok || appErr.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %v", err)
	}
}

func TestWriteAuthError_StatusMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "forbidden", err: apperrors.Forbidden("nope"), want: http.StatusForbidden},
		{name: "unauthorized", err: apperrors.Unauthorized("no creds"), want: http.StatusUnauthorized},
		{name: "plain error defaults 401", err: errors.New("boom"), want: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			writeAuthError(rec, tc.err)
			if rec.Code != tc.want {
				t.Fatalf("expected status %d, got %d", tc.want, rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("expected problem+json, got %q", ct)
			}
		})
	}
}

func TestIdentityFromContext(t *testing.T) {
	t.Parallel()

	if _, ok := IdentityFromContext(context.Background()); ok {
		t.Fatal("expected no identity on bare context")
	}
	ctx := withIdentity(context.Background(), "user-9")
	got, ok := IdentityFromContext(ctx)
	if !ok || got != "user-9" {
		t.Fatalf("expected identity user-9, got %v (ok=%v)", got, ok)
	}
}

// TestBearerAuthenticator_NilValidator verifies a nil validator is a fail-closed
// wiring error: construction succeeds but every request is rejected with 401
// rather than panicking on the request path.
func TestBearerAuthenticator_NilValidator(t *testing.T) {
	t.Parallel()

	auth := BearerAuthenticator(nil)
	r := httptest.NewRequest(http.MethodGet, "/events", http.NoBody)
	r.Header.Set("Authorization", "Bearer any-token")

	identity, err := auth.Authenticate(r)
	if identity != nil {
		t.Fatalf("expected nil identity from nil validator, got %v", identity)
	}
	appErr, ok := apperrors.AsAppError(err)
	if !ok || appErr.HTTPStatus != http.StatusUnauthorized {
		t.Fatalf("expected 401 from nil validator, got %v", err)
	}
}

// TestWriteAuthError_NoMessageLeak verifies the response body never echoes an
// injected error's message: only the canonical 401/403 problem detail is written.
func TestWriteAuthError_NoMessageLeak(t *testing.T) {
	t.Parallel()

	secret := "super-secret-token-abc123"
	rec := httptest.NewRecorder()
	if err := writeAuthError(rec, apperrors.Unauthorized(secret)); err != nil {
		t.Fatalf("writeAuthError returned error: %v", err)
	}
	if body := rec.Body.String(); strings.Contains(body, secret) {
		t.Fatalf("response body leaked the error message: %q", body)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestWriteAuthError_BearerChallenge verifies a bearer rejection advertises a
// `WWW-Authenticate: Bearer` challenge on the 401, while a non-bearer 401 and any
// 403 carry no challenge.
func TestWriteAuthError_BearerChallenge(t *testing.T) {
	t.Parallel()

	auth := BearerAuthenticator(stubValidator{claims: "u"})
	r := httptest.NewRequest(http.MethodGet, "/events", http.NoBody) // missing header
	_, bearerErr := auth.Authenticate(r)

	rec := httptest.NewRecorder()
	if err := writeAuthError(rec, bearerErr); err != nil {
		t.Fatalf("writeAuthError returned error: %v", err)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("expected WWW-Authenticate: Bearer, got %q", got)
	}

	recPlain := httptest.NewRecorder()
	_ = writeAuthError(recPlain, apperrors.Unauthorized("no creds"))
	if got := recPlain.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("non-bearer 401 must not advertise a challenge, got %q", got)
	}

	recForbidden := httptest.NewRecorder()
	_ = writeAuthError(recForbidden, bearerReject(apperrors.Forbidden("nope")))
	if got := recForbidden.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("403 must not advertise a bearer challenge, got %q", got)
	}
}

// TestBearerAuthenticator_PreservesCause verifies the validator's original error
// is preserved as the cause so callers can errors.Is/As it for diagnostics, even
// though the response path redacts it.
func TestBearerAuthenticator_PreservesCause(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("token expired 2020-01-01")
	auth := BearerAuthenticator(stubValidator{err: sentinel})
	r := httptest.NewRequest(http.MethodGet, "/events", http.NoBody)
	r.Header.Set("Authorization", "Bearer some-token")

	_, err := auth.Authenticate(r)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the validator cause to be preserved, got %v", err)
	}
}
