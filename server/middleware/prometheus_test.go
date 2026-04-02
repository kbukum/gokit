package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/server/middleware"
)

func TestPrometheusMetrics_PassesThrough(t *testing.T) {
	handler := middleware.PrometheusMetrics("test-service")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/test", http.NoBody)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "hello" {
		t.Errorf("expected body 'hello', got %q", rr.Body.String())
	}
}

func TestPrometheusMetrics_CapturesStatusCode(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := middleware.PrometheusMetrics("test")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))

			rr := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(context.Background(), "GET", "/", http.NoBody)
			handler.ServeHTTP(rr, req)

			if rr.Code != tt.status {
				t.Errorf("expected %d, got %d", tt.status, rr.Code)
			}
		})
	}
}

func TestPrometheusMetrics_HandlesWriteBody(t *testing.T) {
	handler := middleware.PrometheusMetrics("test")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("response body content"))
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/data", http.NoBody)
	handler.ServeHTTP(rr, req)

	if rr.Body.String() != "response body content" {
		t.Errorf("unexpected body: %q", rr.Body.String())
	}
}

func TestRegisterMetricsEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	middleware.RegisterMetricsEndpoint(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/metrics", http.NoBody)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 from /metrics, got %d", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct == "" {
		t.Error("expected Content-Type header from /metrics")
	}
}

func TestMetricsWriter_Flush(t *testing.T) {
	flushed := false
	inner := &testFlusher{
		ResponseWriter: httptest.NewRecorder(),
		onFlush:        func() { flushed = true },
	}

	handler := middleware.PrometheusMetrics("test")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(context.Background(), "GET", "/stream", http.NoBody)
	handler.ServeHTTP(inner, req)

	if !flushed {
		t.Error("expected Flush to be delegated")
	}
}

type testFlusher struct {
	http.ResponseWriter
	onFlush func()
}

func (f *testFlusher) Flush() { f.onFlush() }
