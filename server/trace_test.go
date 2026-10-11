package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server"
	"github.com/kbukum/gokit/server/middleware"
)

func TestServerTraceCorrelatesRequestLog(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	var output bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "test", logging.WithWriter(&output))
	if err != nil {
		t.Fatal(err)
	}
	s := server.New(&server.Config{}, log)
	err = s.Trace(middleware.TracingConfig{
		Tracer: provider.Tracer("server"),
		Route:  func(*http.Request) string { return "/service.v1.Service/Call" },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Handle("/service.v1.Service/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})); err != nil {
		t.Fatal(err)
	}
	s.ApplyMiddleware()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/service.v1.Service/Call", http.NoBody)
	req.Header.Set("X-Request-Id", "client-request-sentinel")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "/service.v1.Service/Call" {
		t.Fatalf("spans: %+v", spans)
	}
	traceID := spans[0].SpanContext.TraceID().String()
	if got := rec.Header().Get("X-Request-Id"); got != traceID {
		t.Errorf("X-Request-Id = %q, want trace %q", got, traceID)
	}
	var line map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(raw), &entry) == nil && entry["status"] != nil {
			line = entry
		}
	}
	if line == nil || line["trace_id"] != traceID {
		t.Fatalf("request log not correlated with %s: %s", traceID, output.String())
	}
	if strings.Contains(output.String(), "client-request-sentinel") {
		t.Error("request log carries a client-supplied request ID")
	}
}

func TestServerTraceRequiresTracer(t *testing.T) {
	if err := server.New(&server.Config{}, nil).Trace(middleware.TracingConfig{}); err == nil {
		t.Fatal("Trace accepted a configuration without a tracer")
	}
}

func TestServerTraceRecordsPanicAfterCommittedResponse(t *testing.T) {
	t.Parallel()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	s := server.New(&server.Config{}, logging.NewDefault("test"))
	if err := s.Trace(middleware.TracingConfig{Tracer: provider.Tracer("server")}); err != nil {
		t.Fatal(err)
	}
	if err := s.Handle("/panic", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		panic("synthetic-private-panic")
	})); err != nil {
		t.Fatal(err)
	}
	s.ApplyMiddleware()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/panic", http.NoBody))
	spans := exporter.GetSpans()
	if rec.Code != http.StatusOK || len(spans) != 1 || spans[0].Status.Code != codes.Error || strings.Contains(fmt.Sprint(spans), "private-") {
		t.Fatalf("committed response hid the panic failure: status=%d spans=%+v", rec.Code, spans)
	}
}

func TestServerTraceCorrelatesSafePanicResponse(t *testing.T) {
	t.Parallel()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter), sdktrace.WithoutPanicRecording())
	defer func() { _ = provider.Shutdown(t.Context()) }()
	var output bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "test", logging.WithWriter(&output))
	if err != nil {
		t.Fatal(err)
	}
	s := server.New(&server.Config{}, log)
	if err := s.Trace(middleware.TracingConfig{Tracer: provider.Tracer("server")}); err != nil {
		t.Fatal(err)
	}
	if err := s.Handle("/panic/", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("synthetic-private-panic")
	})); err != nil {
		t.Fatal(err)
	}
	s.ApplyMiddleware()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/panic/private-path?token=private-query", http.NoBody))
	if rec.Code != http.StatusInternalServerError || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("panic response: status=%d headers=%v", rec.Code, rec.Header())
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Status.Code != codes.Error {
		t.Fatalf("expected one failed span: %+v", spans)
	}
	traceID := spans[0].SpanContext.TraceID().String()
	var problem map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if problem["traceId"] != traceID || rec.Header().Get("X-Request-Id") != traceID {
		t.Fatalf("uncorrelated panic response: %v", problem)
	}
	foundStatus := false
	for _, attr := range spans[0].Attributes {
		if attr.Key == "http.response.status_code" {
			foundStatus = attr.Value.AsInt64() == http.StatusInternalServerError
		}
	}
	if !foundStatus {
		t.Error("panic span lacks response status 500")
	}
	foundLog := false
	for _, raw := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["message"] == "HTTP error response" {
			foundLog = true
			if entry["trace_id"] != traceID || entry["code"] != "INTERNAL_ERROR" {
				t.Errorf("uncorrelated panic log: %v", entry)
			}
		}
	}
	if !foundLog || strings.Contains(output.String(), "private-") || strings.Contains(fmt.Sprint(spans), "private-") {
		t.Fatal("panic diagnostics are missing or contain private data")
	}
}
