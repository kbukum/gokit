package observability

import (
	"context"
	"net/http"
	"testing"

	"go.opentelemetry.io/otel/propagation"
)

func TestTraceContextCarriers(t *testing.T) {
	headers := MapCarrier{}
	headers.Set("traceparent", "value")
	if headers.Get("traceparent") != "value" {
		t.Fatal("expected map carrier to store values")
	}
	if len(headers.Keys()) != 1 {
		t.Fatalf("expected one map carrier key, got %d", len(headers.Keys()))
	}

	httpHeaders := HeaderCarrier(http.Header{})
	httpHeaders.Set("traceparent", "value")
	if httpHeaders.Get("traceparent") != "value" {
		t.Fatal("expected header carrier to store values")
	}
	if len(httpHeaders.Keys()) != 1 {
		t.Fatalf("expected one header carrier key, got %d", len(httpHeaders.Keys()))
	}
}

func TestInjectExtractTraceContext(t *testing.T) {
	setTextMapPropagator(t, propagation.TraceContext{})

	ctx, span := StartNamedSpan(context.Background(), "test-tracer", "op")
	defer span.End()

	carrier := MapCarrier{}
	InjectTraceContext(ctx, carrier)

	extracted := ExtractTraceContext(context.Background(), carrier)
	if extracted == nil {
		t.Fatal("expected non-nil extracted context")
	}
}
