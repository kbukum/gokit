package observability

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// ── Concurrent RecordRequest / RecordEnd ────────────────────────────────────

func TestConcurrentRecordRequestStartEnd(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	metrics, err := NewMetrics(meter)
	if err != nil {
		t.Fatalf("creating metrics: %v", err)
	}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			metrics.RecordRequestStart(ctx)
			time.Sleep(time.Millisecond)
			method := fmt.Sprintf("GET /item/%d", id)
			metrics.RecordRequestEnd(ctx, "svc", method, "ok", 5*time.Millisecond)
		}(i)
	}
	wg.Wait()
}

func TestConcurrentRecordOperation(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	metrics, err := NewMetrics(meter)
	if err != nil {
		t.Fatalf("creating metrics: %v", err)
	}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			metrics.RecordOperation(ctx, "svc", fmt.Sprintf("op-%d", id), "ok", time.Millisecond)
		}(i)
	}
	wg.Wait()
}

// ── Span nesting (parent-child) ─────────────────────────────────────────────

func TestSpanNestingParentChild(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	ctx := context.Background()
	ctx, parentSpan := StartSpan(ctx, "parent-op")
	_, childSpan := StartSpan(ctx, "child-op")
	childSpan.End()
	parentSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	child := spans[0]
	parent := spans[1]

	if child.Name != "child-op" {
		t.Errorf("expected child name 'child-op', got %q", child.Name)
	}
	if parent.Name != "parent-op" {
		t.Errorf("expected parent name 'parent-op', got %q", parent.Name)
	}
	if child.Parent.TraceID() != parent.SpanContext.TraceID() {
		t.Error("child span should share trace ID with parent")
	}
	if child.Parent.SpanID() != parent.SpanContext.SpanID() {
		t.Error("child's parent span ID should equal parent's span ID")
	}
}

func TestThreeLevelSpanNesting(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	ctx := context.Background()
	ctx, grandparent := StartSpan(ctx, "grandparent")
	ctx, parent := StartSpan(ctx, "parent")
	_, child := StartSpan(ctx, "child")
	child.End()
	parent.End()
	grandparent.End()

	spans := exporter.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}

	// All spans share the same trace ID
	traceID := spans[0].SpanContext.TraceID()
	for _, s := range spans {
		if s.SpanContext.TraceID() != traceID {
			t.Errorf("span %q has different trace ID", s.Name)
		}
	}
}

// ── Metrics error counter ───────────────────────────────────────────────────

func TestRecordErrorIncrementsCounter(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	metrics, err := NewMetrics(meter)
	if err != nil {
		t.Fatalf("creating metrics: %v", err)
	}

	ctx := context.Background()
	// Should not panic; with noop we just verify no errors
	metrics.RecordError(ctx, "timeout", "database")
	metrics.RecordError(ctx, "connection", "cache")
	metrics.RecordError(ctx, "validation", "handler")
}

func TestConcurrentRecordError(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	metrics, err := NewMetrics(meter)
	if err != nil {
		t.Fatalf("creating metrics: %v", err)
	}

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			metrics.RecordError(context.Background(), fmt.Sprintf("err-%d", id), "comp")
		}(i)
	}
	wg.Wait()
}

// ── OperationContext with all attributes ─────────────────────────────────────

func TestOperationContextAllAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	meter := noop.NewMeterProvider().Meter("test")
	metrics, _ := NewMetrics(meter)

	oc := NewOperationContext("my-service", "create-user", "req-123", "user-456", metrics)
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
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	oc := NewOperationContext("svc", "op", "req-1", "", nil)
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

// ── ServiceHealth concurrent updates ────────────────────────────────────────

func TestServiceHealthSequentialAddManyComponents(t *testing.T) {
	sh := NewServiceHealth("svc", "1.0.0")

	for i := 0; i < 50; i++ {
		status := HealthStatusUp
		if i%3 == 0 {
			status = HealthStatusDegraded
		}
		sh.AddComponent(Health{
			Name:   fmt.Sprintf("component-%d", i),
			Status: status,
		})
	}

	if len(sh.Components) != 50 {
		t.Errorf("expected 50 components, got %d", len(sh.Components))
	}
	// At least one degraded component should make overall degraded
	if sh.Status == HealthStatusUp {
		t.Error("expected status to be degraded after adding degraded components")
	}
}

// ── Config defaults validation ──────────────────────────────────────────────

func TestDefaultTracerConfigAllFields(t *testing.T) {
	cfg := DefaultTracerConfig("my-svc")

	if cfg.ServiceName != "my-svc" {
		t.Errorf("ServiceName: got %q", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "1.0.0" {
		t.Errorf("ServiceVersion: got %q", cfg.ServiceVersion)
	}
	if cfg.Environment != "development" {
		t.Errorf("Environment: got %q", cfg.Environment)
	}
	if cfg.Endpoint != "localhost:4318" {
		t.Errorf("Endpoint: got %q", cfg.Endpoint)
	}
	if !cfg.Insecure {
		t.Error("Insecure should be true")
	}
	if cfg.SampleRate != 1.0 {
		t.Errorf("SampleRate: got %f", cfg.SampleRate)
	}
}

func TestDefaultMeterConfigAllFields(t *testing.T) {
	cfg := DefaultMeterConfig("my-svc")

	if cfg.ServiceName != "my-svc" {
		t.Errorf("ServiceName: got %q", cfg.ServiceName)
	}
	if cfg.ServiceVersion != "1.0.0" {
		t.Errorf("ServiceVersion: got %q", cfg.ServiceVersion)
	}
	if cfg.Environment != "development" {
		t.Errorf("Environment: got %q", cfg.Environment)
	}
	if cfg.Endpoint != "localhost:4318" {
		t.Errorf("Endpoint: got %q", cfg.Endpoint)
	}
	if !cfg.Insecure {
		t.Error("Insecure should be true")
	}
	if cfg.Interval != 15*time.Second {
		t.Errorf("Interval: got %v", cfg.Interval)
	}
}

func TestTracerConfigZeroValue(t *testing.T) {
	var cfg TracerConfig
	if cfg.ServiceName != "" {
		t.Errorf("expected empty ServiceName, got %q", cfg.ServiceName)
	}
	if cfg.SampleRate != 0 {
		t.Errorf("expected zero SampleRate, got %f", cfg.SampleRate)
	}
	if cfg.Insecure {
		t.Error("expected Insecure false for zero value")
	}
}

func TestMeterConfigZeroValue(t *testing.T) {
	var cfg MeterConfig
	if cfg.ServiceName != "" {
		t.Errorf("expected empty ServiceName, got %q", cfg.ServiceName)
	}
	if cfg.Interval != 0 {
		t.Errorf("expected zero Interval, got %v", cfg.Interval)
	}
}

// ── OperationContext span integration ───────────────────────────────────────

func TestOperationContextSpanAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	oc := NewOperationContext("svc", "op", "req-1", "user-1", nil)
	ctx := context.Background()
	ctx, span := oc.StartSpanForOperation(ctx, "test.span")

	// Set additional attributes on the span via context
	SetSpanAttribute(ctx, "custom-key", "custom-value")
	SetSpanAttribute(ctx, "int-key", 42)

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
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	oc := NewOperationContext("svc", "op", "req-1", "", nil)
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

// ── Multiple OperationContexts concurrently ─────────────────────────────────

func TestConcurrentOperationContexts(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())
	otel.SetTracerProvider(tp)

	meter := noop.NewMeterProvider().Meter("test")
	metrics, _ := NewMetrics(meter)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			oc := NewOperationContext("svc", fmt.Sprintf("op-%d", id), fmt.Sprintf("req-%d", id), "", metrics)
			ctx := context.Background()
			ctx, span := oc.StartSpanForOperation(ctx, fmt.Sprintf("span-%d", id))
			time.Sleep(time.Millisecond)
			oc.EndOperation(ctx, span, "ok", nil)
		}(i)
	}
	wg.Wait()

	spans := exporter.GetSpans()
	if len(spans) != goroutines {
		t.Errorf("expected %d spans, got %d", goroutines, len(spans))
	}
}

// ── Health struct fields ────────────────────────────────────────────────────

func TestHealthStructFields(t *testing.T) {
	h := Health{
		Name:    "db",
		Status:  HealthStatusUp,
		Message: "connected",
		Details: map[string]string{"host": "localhost", "port": "5432"},
	}

	if h.Name != "db" {
		t.Errorf("Name: got %q", h.Name)
	}
	if h.Status != HealthStatusUp {
		t.Errorf("Status: got %q", h.Status)
	}
	if h.Message != "connected" {
		t.Errorf("Message: got %q", h.Message)
	}
	if len(h.Details) != 2 {
		t.Errorf("Details length: got %d", len(h.Details))
	}
}

func TestServiceHealthEmptyComponents(t *testing.T) {
	sh := NewServiceHealth("svc", "1.0.0")
	if sh.Status != HealthStatusUp {
		t.Errorf("empty service should be up, got %q", sh.Status)
	}
	if len(sh.Components) != 0 {
		t.Errorf("expected 0 components, got %d", len(sh.Components))
	}
}

func TestServiceHealthMultipleDown(t *testing.T) {
	sh := NewServiceHealth("svc", "1.0.0")
	sh.AddComponent(Health{Name: "a", Status: HealthStatusDown})
	sh.AddComponent(Health{Name: "b", Status: HealthStatusDown})

	if sh.Status != HealthStatusDown {
		t.Errorf("expected 'down', got %q", sh.Status)
	}
}
