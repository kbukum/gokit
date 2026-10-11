package observability

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestInitTracerDoesNotRegisterGlobals(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		if otel.GetTracerProvider() != previousProvider {
			otel.SetTracerProvider(previousProvider)
		}
		if !reflect.DeepEqual(otel.GetTextMapPropagator(), previousPropagator) {
			otel.SetTextMapPropagator(previousPropagator)
		}
	})
	cfg := DefaultTracerConfig("isolated")
	provider, err := InitTracer(t.Context(), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	if otel.GetTracerProvider() != previousProvider || !reflect.DeepEqual(otel.GetTextMapPropagator(), previousPropagator) {
		t.Error("tracer construction mutated process-wide telemetry")
	}
}

func TestDefaultTracerConfig(t *testing.T) {
	cfg := DefaultTracerConfig("test-service")

	if cfg.ServiceName != "test-service" {
		t.Errorf("expected ServiceName 'test-service', got %s", cfg.ServiceName)
	}
	if cfg.Endpoint != "localhost:4318" {
		t.Errorf("expected Endpoint 'localhost:4318', got %s", cfg.Endpoint)
	}
	if cfg.SampleRate != 1.0 {
		t.Errorf("expected SampleRate 1.0, got %f", cfg.SampleRate)
	}
	if !cfg.Insecure {
		t.Error("expected Insecure to be true")
	}
}

func TestDefaultTracerConfigFields(t *testing.T) {
	cfg := DefaultTracerConfig("svc")
	if cfg.ServiceVersion != "1.0.0" {
		t.Errorf("expected ServiceVersion '1.0.0', got %q", cfg.ServiceVersion)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected Environment 'development', got %q", cfg.Environment)
	}
}

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

func TestTracer(t *testing.T) {
	tracer := Tracer(t.Context(), "test-tracer")
	if tracer == nil {
		t.Fatal("expected non-nil tracer")
	}
}

func TestStartSpan(t *testing.T) {
	ctx := context.Background()
	ctx, span := StartSpan(ctx, "test-operation")
	defer span.End()

	if span == nil {
		t.Fatal("expected non-nil span")
	}

	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestHelpersUseInstanceOwnedParent(t *testing.T) {
	t.Parallel()
	for _, helper := range []string{"StartSpan", "StartNamedSpan", "OperationContext"} {
		t.Run(helper, func(t *testing.T) {
			t.Parallel()
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			defer func() { _ = tp.Shutdown(t.Context()) }()
			ctx, parent := tp.Tracer("owned").Start(t.Context(), "parent")
			var child trace.Span
			switch helper {
			case "StartSpan":
				_, child = StartSpan(ctx, "child")
			case "StartNamedSpan":
				childCtx, _ := StartNamedSpan(ctx, "named", "child")
				child = trace.SpanFromContext(childCtx)
			case "OperationContext":
				oc := NewOperationContext(OperationSpec{ServiceName: "owned"})
				_, child = oc.StartSpanForOperation(ctx, "child")
			}
			if !child.IsRecording() || child.SpanContext().SpanID() == parent.SpanContext().SpanID() {
				t.Error("helper did not create a recording child span")
			}
			child.End()
			parent.End()
			spans := exporter.GetSpans()
			if len(spans) != 2 || !spans[0].Parent.Equal(spans[1].SpanContext) {
				t.Fatalf("expected owned parent and child: %+v", spans)
			}
		})
	}
}

func TestWithTracerProviderIsolatesRootOperations(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"first", "second"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			exporter := tracetest.NewInMemoryExporter()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
			ctx := tracerContext(t, tp)
			_, span := StartNamedSpan(ctx, name, "root")
			span.End()
			spans := exporter.GetSpans()
			if len(spans) != 1 || spans[0].Parent.IsValid() || spans[0].InstrumentationScope.Name != name {
				t.Fatalf("root did not use the injected instance: %+v", spans)
			}
		})
	}
	var nilProvider *sdktrace.TracerProvider
	for _, provider := range []trace.TracerProvider{nil, nilProvider} {
		if ctx, err := WithTracerProvider(t.Context(), provider); ctx != nil || err == nil {
			t.Fatal("nil tracer provider was accepted")
		}
	}
}

func TestTraceComponentInjectsRootOperations(t *testing.T) {
	t.Parallel()
	exporter := tracetest.NewInMemoryExporter()
	c, err := NewTraceComponent(DefaultTracerConfig("owned"), componentLogger(t, io.Discard),
		WithTraceExporter(func(context.Context) (sdktrace.SpanExporter, error) { return exporter, nil }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		if err := c.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	ctx, err := WithTracerProvider(t.Context(), c)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatal(err)
	}
	_, span := StartSpan(ctx, "root")
	if !span.IsRecording() {
		t.Error("component injection did not create a recording root")
	}
	span.End()
	if err := c.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	var nilComponent *TraceComponent
	if _, err := WithTracerProvider(t.Context(), nilComponent); err == nil {
		t.Error("typed-nil component was accepted")
	}
}

func TestSpanFromContext(t *testing.T) {
	ctx := context.Background()
	span := SpanFromContext(ctx)
	if span == nil {
		t.Fatal("expected non-nil span (noop)")
	}

	// With a real span
	ctx, s := StartSpan(ctx, "test")
	defer s.End()
	got := SpanFromContext(ctx)
	if got == nil {
		t.Fatal("expected non-nil span from context")
	}
}

func TestSetSpanAttributes(t *testing.T) {
	// Use SDK tracer so span.IsRecording() returns true
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	base := tracerContext(t, tp)

	ctx, span := StartSpan(base, "test-attrs")
	defer span.End()

	// Test all supported types - should not panic
	SetSpanAttributes(ctx,
		StringAttribute("string-key", "value"),
		IntAttribute("int-key", 42),
		Int64Attribute("int64-key", 100),
		Float64Attribute("float-key", 3.14),
		BoolAttribute("bool-key", true),
		StringSliceAttribute("string-slice-key", []string{"a", "b"}),
	)
}

func TestSetSpanAttributesNoSpan(t *testing.T) {
	// With background context (no recording span), should not panic
	ctx := context.Background()
	SetSpanAttributes(ctx, StringAttribute("key", "value"))
}

func TestSetSpanError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	base := tracerContext(t, tp)

	ctx, span := StartSpan(base, "test-error")
	defer span.End()

	SetSpanError(ctx, fmt.Errorf("test error"))
}

func TestSetSpanErrorNoSpan(t *testing.T) {
	ctx := context.Background()
	// Should not panic with background context
	SetSpanError(ctx, fmt.Errorf("no span error"))
}

func TestSpanNameConstants(t *testing.T) {
	if SpanHTTPRequest != "http.request" {
		t.Errorf("expected 'http.request', got %q", SpanHTTPRequest)
	}
	if SpanGRPCCall != "grpc.call" {
		t.Errorf("expected 'grpc.call', got %q", SpanGRPCCall)
	}
	if SpanDBQuery != "db.query" {
		t.Errorf("expected 'db.query', got %q", SpanDBQuery)
	}
}

func TestAttributeKeyConstants(t *testing.T) {
	if AttrServiceName != "service.name" {
		t.Errorf("expected 'service.name', got %q", AttrServiceName)
	}
	if AttrOperationName != "operation.name" {
		t.Errorf("expected 'operation.name', got %q", AttrOperationName)
	}
	if AttrRequestID != "request.id" {
		t.Errorf("expected 'request.id', got %q", AttrRequestID)
	}
}

func TestInitTracer(t *testing.T) {
	cfg := &TracerConfig{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "test",
		Endpoint:       "localhost:4318",
		Insecure:       true,
		SampleRate:     1.0,
	}

	tp, err := InitTracer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InitTracer: %v", err)
	}
	if tp == nil {
		t.Fatal("InitTracer returned nil provider")
	}
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
}

func TestInitTracerSamplingRates(t *testing.T) {
	tests := []struct {
		name       string
		sampleRate float64
	}{
		{"always sample", 1.0},
		{"never sample", 0.0},
		{"ratio based", 0.5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &TracerConfig{
				ServiceName:    "test",
				ServiceVersion: "1.0.0",
				Environment:    "test",
				Endpoint:       "localhost:4318",
				Insecure:       true,
				SampleRate:     tc.sampleRate,
			}
			tp, err := InitTracer(context.Background(), cfg)
			if err != nil {
				t.Fatalf("InitTracer: %v", err)
			}
			if tp == nil {
				t.Fatal("InitTracer returned nil provider")
			}
			t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
		})
	}
}

func TestTracerSamplingFollowsAcceptedParent(t *testing.T) {
	tests := []struct {
		name          string
		sampleRate    float64
		parentSampled bool
	}{
		{"unsampled parent overrides always sample", 1.0, false},
		{"sampled parent overrides never sample", 0.0, true},
		{"sampled parent overrides ratio", 0.5, true},
		{"unsampled parent overrides ratio", 0.5, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultTracerConfig("sampling")
			config.SampleRate = tc.sampleRate
			cfg, err := normalizedTracerConfig(&config)
			if err != nil {
				t.Fatal(err)
			}
			tp, err := newTracerProvider(cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
			flags := trace.TraceFlags(0)
			if tc.parentSampled {
				flags = trace.FlagsSampled
			}
			parent := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
				SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
				TraceFlags: flags,
				Remote:     true,
			})
			ctx := trace.ContextWithRemoteSpanContext(context.Background(), parent)
			_, span := tp.Tracer("sampling").Start(ctx, "child")
			defer span.End()
			if got := span.SpanContext().IsSampled(); got != tc.parentSampled {
				t.Fatalf("sampled = %v, want parent decision %v", got, tc.parentSampled)
			}
		})
	}
}

func TestTracerSamplingAppliesRateToRoots(t *testing.T) {
	for _, tc := range []struct {
		rate float64
		want bool
	}{{1.0, true}, {0.0, false}} {
		config := DefaultTracerConfig("sampling")
		config.SampleRate = tc.rate
		cfg, err := normalizedTracerConfig(&config)
		if err != nil {
			t.Fatal(err)
		}
		tp, err := newTracerProvider(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, span := tp.Tracer("sampling").Start(context.Background(), "root")
		if got := span.SpanContext().IsSampled(); got != tc.want {
			t.Errorf("rate %v: sampled = %v, want %v", tc.rate, got, tc.want)
		}
		span.End()
		_ = tp.Shutdown(context.Background())
	}
}

func TestInitTracerSecure(t *testing.T) {
	cfg := &TracerConfig{
		ServiceName:    "test",
		ServiceVersion: "1.0.0",
		Environment:    "test",
		Endpoint:       "localhost:4318",
		Insecure:       false,
		SampleRate:     1.0,
	}

	tp, err := InitTracer(context.Background(), cfg)
	if err != nil {
		t.Fatalf("InitTracer: %v", err)
	}
	if tp == nil {
		t.Fatal("InitTracer returned nil provider")
	}
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
}

func TestSpanNestingParentChild(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	ctx := tracerContext(t, tp)
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
	ctx := tracerContext(t, tp)
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
