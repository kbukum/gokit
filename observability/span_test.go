package observability

import (
	"context"
	"fmt"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestStartNamedSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	setTracerProvider(t, tp)

	_, span := StartNamedSpan(
		context.Background(),
		"test-tracer",
		"test-operation",
		WithSpanKind(SpanKindConsumer),
		WithSpanAttributes(
			StringAttribute("messaging.system", "kafka"),
			IntAttribute("messaging.kafka.partition", 1),
		),
	)
	span.RecordError(fmt.Errorf("boom"))
	span.SetError("boom")
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "test-operation" {
		t.Errorf("expected span name test-operation, got %q", spans[0].Name)
	}
}

func TestSpanSetAttributesAllKinds(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	setTracerProvider(t, tp)

	kinds := []SpanKind{
		SpanKindInternal,
		SpanKindServer,
		SpanKindClient,
		SpanKindProducer,
		SpanKindConsumer,
		SpanKind(99),
	}
	for _, kind := range kinds {
		_, span := StartNamedSpan(context.Background(), "test-tracer", "op", WithSpanKind(kind))
		span.SetAttributes(StringAttribute("k", "v"), IntAttribute("n", 1))
		span.End()
	}

	if got := len(exporter.GetSpans()); got != len(kinds) {
		t.Fatalf("expected %d spans, got %d", len(kinds), got)
	}
}
