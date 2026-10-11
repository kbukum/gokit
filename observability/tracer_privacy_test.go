package observability

import (
	"context"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracerDoesNotRecordPanicValue(t *testing.T) {
	t.Parallel()
	provider, err := newTracerProvider(DefaultTracerConfig("test"))
	if err != nil {
		t.Fatal(err)
	}
	exporter := tracetest.NewInMemoryExporter()
	provider.RegisterSpanProcessor(sdktrace.NewSimpleSpanProcessor(exporter))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	func() {
		defer func() {
			if recovered := recover(); recovered != "private-panic-sentinel" {
				t.Error("tracing changed panic propagation")
			}
		}()
		_, span := provider.Tracer("test").Start(t.Context(), "test.operation")
		defer span.End()
		panic("private-panic-sentinel")
	}()
	spans := exporter.GetSpans()
	if len(spans) != 1 || len(spans[0].Events) != 0 {
		t.Fatal("SDK teardown exported private panic data")
	}
}
