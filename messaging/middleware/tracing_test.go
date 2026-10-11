package middleware

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/observability"
)

func setupTestTracer(t *testing.T) (context.Context, *tracetest.SpanRecorder) {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	ctx, err := observability.WithTracerProvider(t.Context(), tp)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, sr
}

func TestKafkaHeaderCarrier_GetSetKeys(t *testing.T) {
	c := kafkaHeaderCarrier(map[string]string{"a": "1", "b": "2"})

	if c.Get("a") != "1" {
		t.Errorf("Get(a) = %q, want 1", c.Get("a"))
	}
	if c.Get("missing") != "" {
		t.Errorf("Get(missing) = %q, want empty", c.Get("missing"))
	}

	c.Set("c", "3")
	if c.Get("c") != "3" {
		t.Errorf("Get(c) after Set = %q, want 3", c.Get("c"))
	}

	keys := c.Keys()
	sort.Strings(keys)
	if len(keys) != 3 || keys[0] != "a" || keys[1] != "b" || keys[2] != "c" {
		t.Errorf("Keys() = %v, want [a b c]", keys)
	}
}

func TestInjectExtractTraceContext(t *testing.T) {
	base, sr := setupTestTracer(t)

	// Start a span to get an active trace context.
	ctx, span := observability.Tracer(base, "test").Start(base, "producer-span")
	spanCtx := span.SpanContext()

	// Inject into headers.
	headers := make(map[string]string)
	InjectTraceContext(ctx, headers)
	span.End()

	if len(headers) == 0 {
		t.Fatal("InjectTraceContext produced no headers")
	}

	// Extract back.
	newCtx := ExtractTraceContext(context.Background(), headers)
	remoteSpan := trace.SpanFromContext(newCtx).SpanContext()

	if remoteSpan.TraceID() != spanCtx.TraceID() {
		t.Errorf("TraceID mismatch: %s vs %s", remoteSpan.TraceID(), spanCtx.TraceID())
	}

	_ = sr // keep reference
}

func TestTracingHandler_CreatesSpan(t *testing.T) {
	ctx, sr := setupTestTracer(t)

	handler := func(_ context.Context, _ messaging.Message) error { return nil }
	wrapped := TracingHandler(handler)

	msg := messaging.Message{
		Topic:     "orders",
		Partition: 1,
		Key:       "order-42",
		Headers:   map[string]string{},
	}
	err := wrapped(ctx, msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Name() != "orders consume" {
		t.Errorf("span name = %q, want 'orders consume'", s.Name())
	}
	if s.SpanKind() != trace.SpanKindConsumer {
		t.Errorf("span kind = %v, want Consumer", s.SpanKind())
	}

	attrMap := make(map[string]string)
	for _, a := range s.Attributes() {
		attrMap[string(a.Key)] = a.Value.String()
	}
	if attrMap["messaging.system"] != "messaging" {
		t.Errorf("messaging.system = %q", attrMap["messaging.system"])
	}
	if attrMap["messaging.destination"] != "orders" {
		t.Errorf("messaging.destination = %q", attrMap["messaging.destination"])
	}
}

func TestTracingHandler_KafkaAttributes(t *testing.T) {
	ctx, sr := setupTestTracer(t)

	handler := func(_ context.Context, _ messaging.Message) error { return nil }
	wrapped := TracingHandler(handler, WithMessagingSystem("kafka"), WithTracerName("kafka.consumer"))

	msg := messaging.Message{
		Topic:     "orders",
		Partition: 1,
		Key:       "order-42",
		Headers:   map[string]string{},
	}
	if err := wrapped(ctx, msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	attrMap := make(map[string]string)
	for _, a := range spans[0].Attributes() {
		attrMap[string(a.Key)] = a.Value.String()
	}
	if attrMap["messaging.system"] != "kafka" {
		t.Errorf("messaging.system = %q", attrMap["messaging.system"])
	}
	if attrMap["messaging.kafka.message.key"] != "order-42" {
		t.Errorf("messaging.kafka.message.key = %q", attrMap["messaging.kafka.message.key"])
	}
}

func TestTracingHandler_RecordsError(t *testing.T) {
	ctx, sr := setupTestTracer(t)

	handler := func(_ context.Context, _ messaging.Message) error {
		return errors.New("boom")
	}
	wrapped := TracingHandler(handler)

	msg := messaging.Message{Topic: "events", Headers: map[string]string{}}
	err := wrapped(ctx, msg)
	if err == nil {
		t.Fatal("expected error")
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Status().Code != codes.Error {
		t.Errorf("span status code = %d, want Error(%d)", s.Status().Code, codes.Error)
	}
	if len(s.Events()) == 0 {
		t.Error("expected span events (RecordError)")
	}
}

func TestTracingHandler_CustomSpanName(t *testing.T) {
	ctx, sr := setupTestTracer(t)

	handler := func(_ context.Context, _ messaging.Message) error { return nil }
	wrapped := TracingHandler(handler, WithSpanNameFunc(func(msg messaging.Message) string {
		return "custom:" + msg.Topic
	}))

	msg := messaging.Message{Topic: "orders", Headers: map[string]string{}}
	_ = wrapped(ctx, msg)

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "custom:orders" {
		t.Errorf("span name = %q, want custom:orders", spans[0].Name())
	}
}

func TestTracingHandler_PropagatesContext(t *testing.T) {
	ctx, _ := setupTestTracer(t)

	var receivedCtx context.Context
	handler := func(ctx context.Context, _ messaging.Message) error {
		receivedCtx = ctx
		return nil
	}
	wrapped := TracingHandler(handler)

	msg := messaging.Message{Topic: "t", Headers: map[string]string{}}
	_ = wrapped(ctx, msg)

	span := trace.SpanFromContext(receivedCtx)
	if !span.SpanContext().IsValid() {
		t.Error("expected valid span context in handler's ctx")
	}
}
