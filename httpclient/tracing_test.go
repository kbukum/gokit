package httpclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type traceRoundTrip func(*http.Request) (*http.Response, error)

func (f traceRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func httpTraceFixture(t *testing.T) (trace.Tracer, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	return provider.Tracer("test"), exporter
}

func TestTracingTransportPropagationAndBodyOwnership(t *testing.T) {
	t.Parallel()
	tracer, exporter := httpTraceFixture(t)
	var sent trace.SpanContext
	transport, err := NewTracingTransport(traceRoundTrip(func(r *http.Request) (*http.Response, error) {
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		sent = trace.SpanContextFromContext(ctx)
		if r.Header.Get("Baggage") != "" {
			t.Error("incoming baggage forwarded")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data"))}, nil
	}), TracingConfig{Tracer: tracer, Operation: "events.subscribe"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://test/private-path-sentinel", http.NoBody)
	req.Header.Set("Baggage", "secret=private-sentinel")
	response, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Traceparent") != "" || req.Header.Get("Baggage") == "" {
		t.Error("tracing mutated the caller's request")
	}
	if !sent.IsValid() || len(exporter.GetSpans()) != 0 {
		t.Fatal("missing propagation or span ended before body completion")
	}
	if _, ok := response.Body.(io.Writer); ok {
		t.Fatal("ordinary response body incorrectly exposes writing")
	}
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "events.subscribe" || spans[0].SpanContext.SpanID() != sent.SpanID() {
		t.Fatal("incorrect HTTP client span")
	}
	if strings.Contains(fmt.Sprint(spans), "private-") {
		t.Fatal("private request data entered telemetry")
	}
}

type failedTraceBody struct{ readErr, closeErr error }

func (b failedTraceBody) Read([]byte) (int, error) { return 0, b.readErr }
func (b failedTraceBody) Close() error             { return b.closeErr }

type upgradeTraceBody struct {
	bytes.Buffer
	writeErr error
}

func (b *upgradeTraceBody) Write(p []byte) (int, error) {
	if b.writeErr != nil {
		return 0, b.writeErr
	}
	return b.Buffer.Write(p)
}

func (*upgradeTraceBody) Close() error { return nil }

func TestTracingTransportUpgradeBody(t *testing.T) {
	t.Parallel()
	for _, failedWrite := range []bool{false, true} {
		t.Run(fmt.Sprint(failedWrite), func(t *testing.T) {
			t.Parallel()
			tracer, exporter := httpTraceFixture(t)
			cause := errors.New("private-write-sentinel")
			body := &upgradeTraceBody{}
			if failedWrite {
				body.writeErr = cause
			}
			transport, err := NewTracingTransport(traceRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusSwitchingProtocols, Body: body}, nil
			}), TracingConfig{Tracer: tracer, Operation: "upgrade"})
			if err != nil {
				t.Fatal(err)
			}
			response, err := transport.RoundTrip(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://test", http.NoBody))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			upgraded, ok := response.Body.(io.ReadWriteCloser)
			if !ok {
				t.Fatalf("upgrade body lost writing: %T", response.Body)
			}
			n, err := upgraded.Write([]byte("data"))
			if failedWrite {
				if n != 0 || !errors.Is(err, cause) {
					t.Fatalf("write error changed: n=%d err=%v", n, err)
				}
			} else {
				if n != 4 || err != nil || len(exporter.GetSpans()) != 0 {
					t.Fatalf("write ended span or changed output: n=%d err=%v", n, err)
				}
				if data, err := io.ReadAll(upgraded); err != nil || string(data) != "data" {
					t.Fatalf("upgrade read: data=%q err=%v", data, err)
				}
			}
			if err := upgraded.Close(); err != nil {
				t.Fatal(err)
			}
			spans := exporter.GetSpans()
			if len(spans) != 1 || (spans[0].Status.Code == codes.Error) != failedWrite || strings.Contains(fmt.Sprint(spans), "private-") {
				t.Fatalf("incorrect or unsafe upgrade span: %+v", spans)
			}
		})
	}
}

func TestTracingTransportFailures(t *testing.T) {
	t.Parallel()
	cause := errors.New("private-error-sentinel")
	for _, outcome := range []string{"transport", "read", "close", "abandon", "status", "empty", "nil-body", "nil-response"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			tracer, exporter := httpTraceFixture(t)
			transport, err := NewTracingTransport(traceRoundTrip(func(*http.Request) (*http.Response, error) {
				response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("data"))}
				switch outcome {
				case "transport":
					return nil, cause
				case "read":
					response.Body = failedTraceBody{readErr: cause}
				case "close":
					response.Body = failedTraceBody{closeErr: cause}
				case "status":
					response.StatusCode = http.StatusForbidden
				case "empty":
					response.Body = http.NoBody
				case "nil-body":
					response.Body = nil
				case "nil-response":
					return nil, nil //nolint:nilnil // Deliberately broken transport: the wrapper must reject it.
				}
				return response, nil
			}), TracingConfig{Tracer: tracer, Operation: "test.operation"})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://test", http.NoBody)
			req.Header = nil
			response, err := transport.RoundTrip(req)
			switch outcome {
			case "transport":
				if !errors.Is(err, cause) {
					t.Fatal("transport error changed", err)
				}
			case "nil-response":
				if err == nil {
					t.Fatal("invalid transport result accepted")
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				if response.Body != nil {
					if outcome != "abandon" && outcome != "close" {
						_, readErr := io.ReadAll(response.Body)
						if outcome == "read" && !errors.Is(readErr, cause) {
							t.Fatal("read error changed", readErr)
						}
					}
					closeErr := response.Body.Close()
					if outcome == "close" && !errors.Is(closeErr, cause) {
						t.Fatal("close error changed", closeErr)
					}
				}
			}
			spans := exporter.GetSpans()
			if len(spans) != 1 || strings.Contains(fmt.Sprint(spans), "private-") {
				t.Fatal("incorrect or unsafe span output")
			}
			wantError := outcome != "empty" && outcome != "nil-body"
			if (spans[0].Status.Code == codes.Error) != wantError {
				t.Fatal("incorrect HTTP span status")
			}
		})
	}
}

type idleTraceTransport struct{ closed bool }

func (*idleTraceTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}
func (t *idleTraceTransport) CloseIdleConnections() { t.closed = true }

func TestTracingTransportValidationAndCleanup(t *testing.T) {
	t.Parallel()
	tracer, _ := httpTraceFixture(t)
	next := &idleTraceTransport{}
	cfg := TracingConfig{Tracer: tracer, Operation: "test.operation"}
	for _, input := range []struct {
		next http.RoundTripper
		cfg  TracingConfig
	}{
		{nil, cfg}, {next, TracingConfig{Operation: "test"}}, {next, TracingConfig{Tracer: tracer}},
	} {
		if _, err := NewTracingTransport(input.next, input.cfg); err == nil {
			t.Fatal("invalid tracing configuration accepted")
		}
	}
	transport, err := NewTracingTransport(next, cfg)
	if err != nil {
		t.Fatal(err)
	}
	response, err := transport.RoundTrip(nil)
	if response != nil && response.Body != nil {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}
	if err == nil {
		t.Fatal("nil request accepted")
	}
	client := &http.Client{Transport: transport}
	if next.closed {
		t.Fatal("construction closed borrowed transport")
	}
	client.CloseIdleConnections()
	if !next.closed {
		t.Fatal("wrapper hid explicit idle connection cleanup")
	}
}
