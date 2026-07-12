package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/security"
)

func TestSecurityHeaders_AppliesSecureDefaults(t *testing.T) {
	mw, err := SecurityHeaders(nil)
	if err != nil {
		t.Fatalf("SecurityHeaders: %v", err)
	}
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	want := []string{
		"Content-Security-Policy",
		"X-Content-Type-Options",
		"Referrer-Policy",
		"Permissions-Policy",
		"X-Frame-Options",
		"Strict-Transport-Security",
	}
	for _, key := range want {
		if got := w.Header().Get(key); got == "" {
			t.Errorf("missing secure header %q", key)
		}
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

func TestSecurityHeaders_DisabledEmitsNothing(t *testing.T) {
	mw, err := SecurityHeaders(&security.HeadersConfig{Disabled: true})
	if err != nil {
		t.Fatalf("SecurityHeaders: %v", err)
	}
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Security-Policy"); got != "" {
		t.Errorf("disabled config should not set CSP, got %q", got)
	}
}

func TestSecurityHeaders_InvalidConfigErrors(t *testing.T) {
	_, err := SecurityHeaders(&security.HeadersConfig{XFrameOptions: "BOGUS"})
	if err == nil {
		t.Fatal("expected error for invalid X-Frame-Options")
	}
}
