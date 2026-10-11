package middleware_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server/middleware"
)

func setupTestTracer(t *testing.T) (*tracetest.InMemoryExporter, trace.Tracer) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			t.Errorf("shutting down tracer: %v", err)
		}
	})
	return exporter, tp.Tracer("test-service")
}

func tracingMiddleware(t *testing.T, cfg middleware.TracingConfig) middleware.Middleware {
	t.Helper()
	mw, err := middleware.Tracing(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return mw
}

func TestTracingPublicInputDoesNotControlCorrelation(t *testing.T) {
	t.Parallel()
	exporter, tracer := setupTestTracer(t)
	handler := tracingMiddleware(t, middleware.TracingConfig{Tracer: tracer})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if baggage.FromContext(r.Context()).Len() != 0 {
			t.Error("incoming baggage reached handler")
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/private-resource-sentinel?token=private-query-sentinel", http.NoBody)
	req.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00")
	req.Header.Set("X-Forwarded-Proto", "private-scheme-sentinel")
	req.Header.Set("X-Request-Id", "private-request-sentinel")
	req.Header.Set("Baggage", "token=private-baggage-sentinel")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d; public input must not suppress local sampling", len(spans))
	}
	if spans[0].SpanContext.TraceID().String() == "4bf92f3577b34da6a3ce929d0e0e4736" || spans[0].Parent.IsValid() {
		t.Error("public caller selected the internal trace")
	}
	if spans[0].Name != "http.request" || strings.Contains(fmt.Sprint(spans[0]), "private-") {
		t.Error("untrusted request value leaked into telemetry")
	}
}

func TestTracingTrustedParentAndResponse(t *testing.T) {
	t.Parallel()
	exporter, tracer := setupTestTracer(t)
	handler := tracingMiddleware(t, middleware.TracingConfig{
		Tracer: tracer,
		Propagator: propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{}),
		TrustParent: func(*http.Request) bool { return true },
		Route:       func(*http.Request) string { return "/example.Service/Call" },
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if baggage.FromContext(r.Context()).Len() != 0 {
			t.Error("incoming baggage reached handler")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/example.Service/Call", http.NoBody)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	req.Header.Set("Baggage", "secret=private-sentinel")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	span := spans[0]
	if span.Name != "/example.Service/Call" || span.SpanKind != trace.SpanKindServer ||
		span.SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" ||
		span.Parent.SpanID().String() != "00f067aa0ba902b7" || !span.Parent.IsRemote() {
		t.Fatalf("incorrect trusted parent or canonical name: %+v", span)
	}
	response := propagation.TraceContext{}.Extract(t.Context(), propagation.HeaderCarrier(rr.Header()))
	if got := trace.SpanContextFromContext(response); got.TraceID() != span.SpanContext.TraceID() || got.SpanID() != span.SpanContext.SpanID() {
		t.Error("response trace does not identify server span")
	}
	if rr.Header().Get("Baggage") != "" || rr.Header().Get("X-Request-Id") != span.SpanContext.TraceID().String() {
		t.Error("response correlation or baggage is incorrect")
	}
	attrs := make(map[string]string)
	for _, a := range span.Attributes {
		attrs[string(a.Key)] = a.Value.String()
	}
	for key, want := range map[string]string{
		"http.request.method": "POST", "http.route": "/example.Service/Call",
		"http.response.status_code": "201", "url.scheme": "https",
	} {
		if attrs[key] != want {
			t.Errorf("%s = %q, want %q", key, attrs[key], want)
		}
	}
}

func TestTracingStatusAndInstanceIsolation(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			exporter, tracer := setupTestTracer(t)
			other, _ := setupTestTracer(t)
			handler := tracingMiddleware(t, middleware.TracingConfig{Tracer: tracer})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			req := httptest.NewRequestWithContext(t.Context(), "private-method-sentinel", "/", http.NoBody)
			req.Header.Set("X-Forwarded-Proto", "https")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			spans := exporter.GetSpans()
			if rr.Code != status || len(spans) != 1 || len(other.GetSpans()) != 0 {
				t.Fatal("response changed or providers shared state")
			}
			if (spans[0].Status.Code == codes.Error) != (status >= 500) {
				t.Fatal("wrong span status")
			}
			for _, a := range spans[0].Attributes {
				if a.Key == "url.scheme" && a.Value.AsString() != "http" {
					t.Error("forwarded header selected scheme")
				}
				if a.Key == "http.request.method" && a.Value.AsString() != "_OTHER" {
					t.Error("unbounded method attribute")
				}
			}
		})
	}
}

func TestTracingRequiresTracer(t *testing.T) {
	t.Parallel()
	if mw, err := middleware.Tracing(middleware.TracingConfig{}); err == nil || mw != nil {
		t.Fatal("missing tracer accepted")
	}
}

func TestTracingRequestLogCorrelation(t *testing.T) {
	t.Parallel()
	exporter, tracer := setupTestTracer(t)
	var output bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "test", logging.WithWriter(&output))
	if err != nil {
		t.Fatal(err)
	}
	handler := tracingMiddleware(t, middleware.TracingConfig{Tracer: tracer})(
		middleware.RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})),
	)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/private-path-sentinel?token=private-query-sentinel", http.NoBody)
	req.Header.Set("X-Request-Id", "private-request-sentinel")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	text := output.String()
	if strings.Contains(text, "private-") {
		t.Error("request input entered log fields")
	}
	if !strings.Contains(text, spans[0].SpanContext.TraceID().String()) ||
		!strings.Contains(text, spans[0].SpanContext.SpanID().String()) {
		t.Error("request logs lack trace/span correlation")
	}
}
