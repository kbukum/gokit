package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kbukum/gokit/server/middleware"
)

func TestTimeout_ExceededReturns503(t *testing.T) {
	t.Parallel()

	h := middleware.Timeout(10 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/slow", http.NoBody)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type: got %q want application/problem+json", ct)
	}
	if !json.Valid(rr.Body.Bytes()) {
		t.Fatalf("timeout body is not valid JSON: %q", rr.Body.String())
	}
}

func TestTimeout_WithinDeadlinePasses(t *testing.T) {
	t.Parallel()

	h := middleware.Timeout(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/fast", http.NoBody)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type passthrough: got %q want application/json", ct)
	}
	if rr.Body.String() != `{"ok":true}` {
		t.Fatalf("body passthrough: got %q", rr.Body.String())
	}
}

func TestTimeout_NonPositiveDisables(t *testing.T) {
	t.Parallel()

	called := false
	h := middleware.Timeout(0)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called || rr.Code != http.StatusTeapot {
		t.Fatalf("disabled timeout must pass through unchanged; called=%v code=%d", called, rr.Code)
	}
}
