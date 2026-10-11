package connect

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type traceStreamStub struct {
	connectrpc.StreamingClientConn
	header                           http.Header
	sendErr, receiveErr, closeReqErr error
	closeRespErr                     error
}

func (s *traceStreamStub) RequestHeader() http.Header { return s.header }
func (s *traceStreamStub) Send(any) error             { return s.sendErr }
func (s *traceStreamStub) Receive(any) error          { return s.receiveErr }
func (s *traceStreamStub) CloseRequest() error        { return s.closeReqErr }
func (s *traceStreamStub) CloseResponse() error       { return s.closeRespErr }

func newTraceStream(t *testing.T, stub *traceStreamStub) (connectrpc.StreamingClientConn, *tracetest.InMemoryExporter) {
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
	interceptor, err := TracingInterceptor(TracingConfig{Tracer: provider.Tracer("test")})
	if err != nil {
		t.Fatal(err)
	}
	conn := interceptor.WrapStreamingClient(func(context.Context, connectrpc.Spec) connectrpc.StreamingClientConn {
		return stub
	})(t.Context(), connectrpc.Spec{IsClient: true, Procedure: "/test.Service/Watch", StreamType: connectrpc.StreamTypeServer})
	return conn, exporter
}

func TestTracingStreamLifetime(t *testing.T) {
	t.Parallel()
	stub := &traceStreamStub{header: make(http.Header)}
	conn, exporter := newTraceStream(t, stub)
	conn.RequestHeader().Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	conn.RequestHeader().Set("Baggage", "secret=private-sentinel")
	if err := conn.Send(nil); err != nil {
		t.Fatal(err)
	}
	ctx := propagation.TraceContext{}.Extract(t.Context(), propagation.HeaderCarrier(stub.header))
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() || sc.TraceID().String() == "4bf92f3577b34da6a3ce929d0e0e4736" || stub.header.Get("Baggage") != "" {
		t.Fatal("caller headers replaced client propagation")
	}
	if err := conn.CloseRequest(); err != nil {
		t.Fatal(err)
	}
	if err := conn.Receive(nil); err != nil || len(exporter.GetSpans()) != 0 {
		t.Fatal("request close or a message prematurely ended the span")
	}
	stub.receiveErr = io.EOF
	if err := conn.Receive(nil); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if err := conn.CloseResponse(); err != nil {
		t.Fatal(err)
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Status.Code == codes.Error || spans[0].SpanContext.SpanID() != sc.SpanID() {
		t.Fatal("stream did not end exactly once with its propagated span")
	}
}

func TestTracingStreamFailureAndAbandonment(t *testing.T) {
	t.Parallel()
	denied := connectrpc.NewError(connectrpc.CodePermissionDenied, errors.New("private-error-sentinel"))
	for _, outcome := range []string{"send", "receive", "close-request", "close-response", "abandon", "send-eof", "concurrent"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			stub := &traceStreamStub{header: make(http.Header)}
			conn, exporter := newTraceStream(t, stub)
			var err error
			switch outcome {
			case "send":
				stub.sendErr = denied
				err = conn.Send(nil)
			case "receive":
				stub.receiveErr = denied
				err = conn.Receive(nil)
			case "close-request":
				stub.closeReqErr = denied
				err = conn.CloseRequest()
			case "close-response":
				stub.closeRespErr = denied
				err = conn.CloseResponse()
			case "abandon":
				err = conn.CloseResponse()
			case "send-eof":
				stub.sendErr = io.EOF
				if err := conn.Send(nil); !errors.Is(err, io.EOF) || len(exporter.GetSpans()) != 0 {
					t.Fatal("send EOF hid the response's terminal error")
				}
				stub.receiveErr = denied
				err = conn.Receive(nil)
			case "concurrent":
				stub.sendErr, stub.receiveErr = denied, denied
				var workers sync.WaitGroup
				workers.Go(func() {
					if err := conn.Send(nil); !errors.Is(err, denied) {
						t.Error(err)
					}
				})
				workers.Go(func() {
					if err := conn.Receive(nil); !errors.Is(err, denied) {
						t.Error(err)
					}
				})
				workers.Wait()
				err = denied
			}
			if outcome != "abandon" && !errors.Is(err, denied) {
				t.Fatal("stream failure changed", err)
			}
			if outcome == "abandon" && err != nil {
				t.Fatal("tracing changed close result", err)
			}
			spans := exporter.GetSpans()
			if len(spans) != 1 || spans[0].Status.Code != codes.Error {
				t.Fatal("failed or abandoned stream not recorded exactly once as an error")
			}
			if spans[0].Status.Description == "private-error-sentinel" {
				t.Fatal("error text leaked into span")
			}
		})
	}
}

func TestTracingInterceptorRequiresTracer(t *testing.T) {
	t.Parallel()
	if interceptor, err := TracingInterceptor(TracingConfig{}); err == nil || interceptor != nil {
		t.Fatal("missing tracer accepted")
	}
}
