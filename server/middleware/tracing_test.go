package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/kbukum/gokit/server/middleware"
)

func setupTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	return exporter
}

func TestTracing_CreatesSpan(t *testing.T) {
	exporter := setupTestTracer(t)

	handler := middleware.Tracing("test-service")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/api/test", http.NoBody)
	handler.ServeHTTP(rr, req)

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	if spans[0].Name != "GET /api/test" {
		t.Errorf("expected span name 'GET /api/test', got %q", spans[0].Name)
	}
}

func TestTracing_SetsHTTPAttributes(t *testing.T) {
	exporter := setupTestTracer(t)

	handler := middleware.Tracing("test-service")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/users", http.NoBody)
	handler.ServeHTTP(rr, req)

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	attrs := make(map[string]interface{})
	for _, a := range spans[0].Attributes {
		switch a.Value.Type().String() {
		case "STRING":
			attrs[string(a.Key)] = a.Value.AsString()
		case "INT64":
			attrs[string(a.Key)] = a.Value.AsInt64()
		}
	}

	if attrs["http.method"] != "POST" {
		t.Errorf("expected http.method=POST, got %v", attrs["http.method"])
	}
	if attrs["http.target"] != "/users" {
		t.Errorf("expected http.target=/users, got %v", attrs["http.target"])
	}
	if attrs["http.status_code"] != int64(201) {
		t.Errorf("expected http.status_code=201, got %v", attrs["http.status_code"])
	}
	if attrs["http.scheme"] != "http" {
		t.Errorf("expected http.scheme=http, got %v", attrs["http.scheme"])
	}
}

func TestTracing_SetsErrorStatusOn5xx(t *testing.T) {
	exporter := setupTestTracer(t)

	handler := middleware.Tracing("test-service")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/fail", http.NoBody)
	handler.ServeHTTP(rr, req)

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	if spans[0].Status.Code.String() != "Error" {
		t.Errorf("expected span status Error for 500, got %s", spans[0].Status.Code)
	}
}

func TestTracing_NoErrorStatusOnSuccess(t *testing.T) {
	exporter := setupTestTracer(t)

	handler := middleware.Tracing("test-service")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/ok", http.NoBody)
	handler.ServeHTTP(rr, req)

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	if spans[0].Status.Code.String() == "Error" {
		t.Error("expected no error status for 200")
	}
}

func TestTracing_InjectsTraceContextInResponse(t *testing.T) {
	setupTestTracer(t)

	handler := middleware.Tracing("test-service")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/", http.NoBody)
	handler.ServeHTTP(rr, req)

	tp := rr.Header().Get("Traceparent")
	if tp == "" {
		t.Error("expected Traceparent header in response")
	}
}

func TestTracing_ExtractsIncomingTraceContext(t *testing.T) {
	exporter := setupTestTracer(t)

	handler := middleware.Tracing("test-service")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/", http.NoBody)
	// Inject a valid W3C traceparent header.
	req.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	handler.ServeHTTP(rr, req)

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	traceID := spans[0].SpanContext.TraceID().String()
	if traceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("expected trace ID from incoming header, got %s", traceID)
	}
}

func TestTracing_PreservesResponseStatus(t *testing.T) {
	setupTestTracer(t)

	handler := middleware.Tracing("test-service")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/missing", http.NoBody)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestTracing_XForwardedProtoScheme(t *testing.T) {
	exporter := setupTestTracer(t)

	handler := middleware.Tracing("test-service")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(context.Background(), "GET", "/", http.NoBody)
	req.Header.Set("X-Forwarded-Proto", "https")
	handler.ServeHTTP(rr, req)

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one span")
	}

	for _, a := range spans[0].Attributes {
		if string(a.Key) == "http.scheme" && a.Value.AsString() != "https" {
			t.Errorf("expected http.scheme=https, got %s", a.Value.AsString())
		}
	}
}
