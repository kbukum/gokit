package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	apperrors "github.com/kbukum/gokit/errors"
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

type countingAuthenticator struct{ calls int }

func (a *countingAuthenticator) Authenticate(r *http.Request) (identity string, present bool, err error) {
	a.calls++
	switch r.Header.Get("X-Key") {
	case "":
		return "", false, nil
	case "good":
		return "billing", true, nil
	default:
		return "", true, apperrors.Unauthorized("")
	}
}

type callerKey struct{}

// One authentication per request decides both trace trust and access; a rejected request gets a fresh root trace whose ID its rejection carries.
func TestServerAuthenticatesOnceForAccessAndTraceTrust(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() { _ = provider.Shutdown(t.Context()) }()
	auth := &countingAuthenticator{}
	stages, err := middleware.NewAuthStages[string](auth, func(ctx context.Context, caller string) context.Context {
		return context.WithValue(ctx, callerKey{}, caller)
	})
	if err != nil {
		t.Fatal(err)
	}
	s := server.New(&server.Config{}, nil)
	if err := s.Authenticate(nil); err == nil {
		t.Fatal("nil authentication accepted")
	}
	if err := s.Authenticate(stages.Authenticate); err != nil {
		t.Fatal(err)
	}
	if err := s.Trace(middleware.TracingConfig{Tracer: provider.Tracer("server"), TrustParent: stages.Trusted}); err != nil {
		t.Fatal(err)
	}
	var served []string
	if err := s.Handle("/svc/", stages.Enforce(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = append(served, r.Context().Value(callerKey{}).(string))
	}))); err != nil {
		t.Fatal(err)
	}
	s.ApplyMiddleware()

	const parentTrace = "4bf92f3577b34da6a3ce929d0e0e4736"
	call := func(key string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/svc/Call", http.NoBody)
		req.Header.Set("Traceparent", "00-"+parentTrace+"-00f067aa0ba902b7-01")
		if key != "" {
			req.Header.Set("X-Key", key)
		}
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec
	}
	for _, tc := range []struct {
		key     string
		status  int
		trusted bool
	}{{"good", http.StatusOK, true}, {"bad", http.StatusUnauthorized, false}, {"", http.StatusUnauthorized, false}} {
		exporter.Reset()
		before := auth.calls
		rec := call(tc.key)
		spans := exporter.GetSpans()
		if rec.Code != tc.status || len(spans) != 1 || auth.calls-before != 1 {
			t.Fatalf("key %q: status=%d spans=%d authentications=%d", tc.key, rec.Code, len(spans), auth.calls-before)
		}
		traceID := spans[0].SpanContext.TraceID().String()
		if (traceID == parentTrace) != tc.trusted {
			t.Fatalf("key %q: trace %s, trusted=%v", tc.key, traceID, tc.trusted)
		}
		if rec.Header().Get("X-Request-Id") != traceID {
			t.Fatalf("key %q: response carries %q, not trace %s", tc.key, rec.Header().Get("X-Request-Id"), traceID)
		}
	}
	if len(served) != 1 || served[0] != "billing" {
		t.Fatalf("served = %v", served)
	}
}
