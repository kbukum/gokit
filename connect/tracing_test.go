package connect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestTracingUnaryRoundTrip(t *testing.T) {
	t.Parallel()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	interceptor, err := TracingInterceptor(TracingConfig{Tracer: provider.Tracer("test")})
	if err != nil {
		t.Fatal(err)
	}
	const procedure = "/test.Service/Call"
	handler := connectrpc.NewUnaryHandler(procedure, func(context.Context, *connectrpc.Request[emptypb.Empty]) (*connectrpc.Response[emptypb.Empty], error) {
		return connectrpc.NewResponse(&emptypb.Empty{}), nil
	}, connectrpc.WithInterceptors(interceptor))
	// The in-process adapter proves interceptor composition, not network integration.
	client := connectrpc.NewClient[emptypb.Empty, emptypb.Empty](
		traceHTTPClient{handler}, "http://test"+procedure, connectrpc.WithInterceptors(interceptor),
	)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if _, err := client.CallUnary(ctx, connectrpc.NewRequest(&emptypb.Empty{})); err != nil {
		t.Fatal(err)
	}
	if got := exporter.GetSpans(); len(got) != 2 {
		t.Fatalf("spans = %d, want client and server", len(got))
	}
}

type traceHTTPClient struct{ handler http.Handler }

func (c traceHTTPClient) Do(req *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	c.handler.ServeHTTP(w, req)
	return w.Result(), nil
}
