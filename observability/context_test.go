package observability

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewOperationContext(t *testing.T) {
	oc := NewOperationContext(OperationSpec{ServiceName: "backend", OperationName: "create-user", RequestID: "req-1", UserID: "user-1", Metrics: nil})

	if oc.ServiceName != "backend" {
		t.Errorf("expected ServiceName 'backend', got %s", oc.ServiceName)
	}
	if oc.OperationName != "create-user" {
		t.Errorf("expected OperationName 'create-user', got %s", oc.OperationName)
	}
	if oc.RequestID != "req-1" {
		t.Errorf("expected RequestID 'req-1', got %s", oc.RequestID)
	}
	if oc.UserID != "user-1" {
		t.Errorf("expected UserID 'user-1', got %s", oc.UserID)
	}
	if oc.StartTime.IsZero() {
		t.Error("expected StartTime to be set")
	}
}

func TestOperationContextFromContext(t *testing.T) {
	oc := NewOperationContext(OperationSpec{ServiceName: "backend", OperationName: "create-user", RequestID: "req-1", UserID: "user-1", Metrics: nil})
	ctx := WithOperationContext(context.Background(), oc)

	retrieved := OperationContextFromContext(ctx)
	if retrieved == nil {
		t.Fatal("expected operation context from context")
	}
	if retrieved.ServiceName != oc.ServiceName {
		t.Errorf("expected ServiceName %s, got %s", oc.ServiceName, retrieved.ServiceName)
	}
}

func TestOperationContextFromContext_NotSet(t *testing.T) {
	retrieved := OperationContextFromContext(context.Background())
	if retrieved != nil {
		t.Error("expected nil when operation context not set")
	}
}

func TestOperationContext_Duration(t *testing.T) {
	oc := NewOperationContext(OperationSpec{ServiceName: "backend", OperationName: "create-user", RequestID: "req-1", UserID: "", Metrics: nil})
	oc.StartTime = time.Now().Add(-50 * time.Millisecond)

	duration := oc.Duration()
	if duration < 45*time.Millisecond || duration > 200*time.Millisecond {
		t.Errorf("expected duration around 50ms, got %v", duration)
	}
}

func TestOperationContext_NilMetrics(t *testing.T) {
	oc := NewOperationContext(OperationSpec{ServiceName: "backend", OperationName: "create-user", RequestID: "req-1", UserID: "", Metrics: nil})
	ctx := context.Background()

	ctx, span := oc.StartSpanForOperation(ctx, "test.op")
	oc.EndOperation(ctx, span, "ok", nil)
}

func TestOperationContextWithMetrics(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	metrics, _ := NewMetrics(meter)

	oc := NewOperationContext(OperationSpec{ServiceName: "backend", OperationName: "create-user", RequestID: "req-1", UserID: "user-1", Metrics: metrics})
	ctx := context.Background()

	ctx, span := oc.StartSpanForOperation(ctx, "test.op")
	oc.EndOperation(ctx, span, "ok", nil)
}

func TestOperationContextEndWithError(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	metrics, _ := NewMetrics(meter)

	oc := NewOperationContext(OperationSpec{ServiceName: "backend", OperationName: "create-user", RequestID: "req-1", UserID: "", Metrics: metrics})
	ctx := context.Background()

	ctx, span := oc.StartSpanForOperation(ctx, "test.op")
	oc.EndOperation(ctx, span, "error", fmt.Errorf("something failed"))
}

func TestOperationContextWithMetadata(t *testing.T) {
	oc := NewOperationContext(OperationSpec{ServiceName: "backend", OperationName: "op", RequestID: "req-1", UserID: "", Metrics: nil})
	if oc.Metrics != nil {
		t.Error("expected nil metrics")
	}
}

func TestOperationContextAllAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	setTracerProvider(t, tp)

	meter := noop.NewMeterProvider().Meter("test")
	metrics, _ := NewMetrics(meter)

	oc := NewOperationContext(OperationSpec{ServiceName: "my-service", OperationName: "create-user", RequestID: "req-123", UserID: "user-456", Metrics: metrics})
	ctx := context.Background()
	ctx, span := oc.StartSpanForOperation(ctx, "test.all-attrs")
	oc.EndOperation(ctx, span, "ok", nil)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	attrMap := make(map[string]any)
	for _, a := range s.Attributes {
		attrMap[string(a.Key)] = a.Value.AsInterface()
	}

	if v, ok := attrMap[AttrServiceName]; !ok || v != "my-service" {
		t.Errorf("expected service.name='my-service', got %v", v)
	}
	if v, ok := attrMap[AttrOperationName]; !ok || v != "create-user" {
		t.Errorf("expected operation.name='create-user', got %v", v)
	}
	if v, ok := attrMap[AttrRequestID]; !ok || v != "req-123" {
		t.Errorf("expected request.id='req-123', got %v", v)
	}
	if v, ok := attrMap[AttrUserID]; !ok || v != "user-456" {
		t.Errorf("expected user.id='user-456', got %v", v)
	}
	if _, ok := attrMap[AttrStatus]; !ok {
		t.Error("expected status attribute to be set")
	}
	if _, ok := attrMap[AttrDurationMs]; !ok {
		t.Error("expected duration_ms attribute to be set")
	}
}

func TestOperationContextWithoutUserID(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	setTracerProvider(t, tp)

	oc := NewOperationContext(OperationSpec{ServiceName: "svc", OperationName: "op", RequestID: "req-1", UserID: "", Metrics: nil})
	ctx := context.Background()
	ctx, span := oc.StartSpanForOperation(ctx, "test.no-user")
	oc.EndOperation(ctx, span, "ok", nil)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	for _, a := range spans[0].Attributes {
		if string(a.Key) == AttrUserID {
			t.Error("user.id should not be set when empty")
		}
	}
}

func TestOperationContextSpanAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	setTracerProvider(t, tp)

	oc := NewOperationContext(OperationSpec{ServiceName: "svc", OperationName: "op", RequestID: "req-1", UserID: "user-1", Metrics: nil})
	ctx := context.Background()
	ctx, span := oc.StartSpanForOperation(ctx, "test.span")

	// Set additional attributes on the span via context
	SetSpanAttributes(ctx,
		StringAttribute("custom-key", "custom-value"),
		IntAttribute("int-key", 42),
	)

	oc.EndOperation(ctx, span, "ok", nil)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	found := false
	for _, a := range spans[0].Attributes {
		if string(a.Key) == "custom-key" && a.Value.AsString() == "custom-value" {
			found = true
		}
	}
	if !found {
		t.Error("custom attribute not found on span")
	}
}

func TestOperationContextEndWithErrorSetsAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	setTracerProvider(t, tp)

	oc := NewOperationContext(OperationSpec{ServiceName: "svc", OperationName: "op", RequestID: "req-1", UserID: "", Metrics: nil})
	ctx := context.Background()
	ctx, span := oc.StartSpanForOperation(ctx, "test.error")
	testErr := fmt.Errorf("database connection failed")
	oc.EndOperation(ctx, span, "error", testErr)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	attrMap := make(map[string]any)
	for _, a := range spans[0].Attributes {
		attrMap[string(a.Key)] = a.Value.AsInterface()
	}

	if v, ok := attrMap[AttrErrorMessage]; !ok || v != "database connection failed" {
		t.Errorf("expected error.message='database connection failed', got %v", v)
	}
	if v, ok := attrMap[AttrStatus]; !ok || v != "error" {
		t.Errorf("expected status='error', got %v", v)
	}
}

func TestConcurrentOperationContexts(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	setTracerProvider(t, tp)

	meter := noop.NewMeterProvider().Meter("test")
	metrics, _ := NewMetrics(meter)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			oc := NewOperationContext(OperationSpec{ServiceName: "svc", OperationName: fmt.Sprintf("op-%d", id), RequestID: fmt.Sprintf("req-%d", id), UserID: "", Metrics: metrics})
			ctx := context.Background()
			ctx, span := oc.StartSpanForOperation(ctx, fmt.Sprintf("span-%d", id))
			oc.EndOperation(ctx, span, "ok", nil)
		}(i)
	}
	wg.Wait()

	spans := exporter.GetSpans()
	if len(spans) != goroutines {
		t.Errorf("expected %d spans, got %d", goroutines, len(spans))
	}
}
