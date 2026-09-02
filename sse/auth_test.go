package sse

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
