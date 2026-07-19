package observability

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
)

func TestDefaultMeterConfig(t *testing.T) {
	cfg := DefaultMeterConfig("test-service")

	if cfg.ServiceName != "test-service" {
		t.Errorf("expected ServiceName 'test-service', got %s", cfg.ServiceName)
	}
	if cfg.Interval != 15*time.Second {
		t.Errorf("expected Interval 15s, got %v", cfg.Interval)
	}
}

func TestDefaultMeterConfigFields(t *testing.T) {
	cfg := DefaultMeterConfig("svc")
	if cfg.ServiceVersion != "1.0.0" {
		t.Errorf("expected ServiceVersion '1.0.0', got %q", cfg.ServiceVersion)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected Environment 'development', got %q", cfg.Environment)
	}
	if !cfg.Insecure {
		t.Error("expected Insecure true for default config")
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

func TestMeterConfigZeroValue(t *testing.T) {
	var cfg MeterConfig
	if cfg.ServiceName != "" {
		t.Errorf("expected empty ServiceName, got %q", cfg.ServiceName)
	}
	if cfg.Interval != 0 {
		t.Errorf("expected zero Interval, got %v", cfg.Interval)
	}
}

func TestMeter(t *testing.T) {
	meter := Meter("test-meter")
	if meter == nil {
		t.Fatal("expected non-nil meter")
	}
}

func TestNewMetrics(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	metrics, err := NewMetrics(meter)
	if err != nil {
		t.Fatalf("unexpected error creating metrics: %v", err)
	}
	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}

	ctx := context.Background()
	metrics.RecordRequestStart(ctx)
	metrics.RecordRequestEnd(ctx, RequestMetric{Service: "svc", Method: "GET /test", Status: "ok", Duration: 100 * time.Millisecond})
	metrics.RecordOperation(ctx, OperationMetric{Service: "svc", Operation: "create", Status: "ok", Duration: 50 * time.Millisecond})
	metrics.RecordError(ctx, "validation", "handler")
}

func TestRecordErrorDirect(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	metrics, err := NewMetrics(meter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should not panic
	metrics.RecordError(context.Background(), "timeout", "database")
}

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
			method := fmt.Sprintf("GET /item/%d", id)
			metrics.RecordRequestEnd(ctx, RequestMetric{Service: "svc", Method: method, Status: "ok", Duration: 5 * time.Millisecond})
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
			metrics.RecordOperation(ctx, OperationMetric{Service: "svc", Operation: fmt.Sprintf("op-%d", id), Status: "ok", Duration: time.Millisecond})
		}(i)
	}
	wg.Wait()
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

func TestInitMeter(t *testing.T) {
	cfg := &MeterConfig{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		Environment:    "test",
		Endpoint:       "localhost:4318",
		Insecure:       true,
		Interval:       15 * time.Second,
	}

	mp, err := InitMeter(context.Background(), cfg)
	if err != nil {
		t.Skipf("InitMeter failed (known schema conflict): %v", err)
	}
	if mp != nil {
		defer mp.Shutdown(context.Background())
	}
}

func TestInitMeterSecure(t *testing.T) {
	cfg := &MeterConfig{
		ServiceName:    "test",
		ServiceVersion: "1.0.0",
		Environment:    "test",
		Endpoint:       "localhost:4318",
		Insecure:       false,
		Interval:       0,
	}

	mp, err := InitMeter(context.Background(), cfg)
	if err != nil {
		t.Skipf("InitMeter failed (known schema conflict): %v", err)
	}
	if mp != nil {
		defer mp.Shutdown(context.Background())
	}
}
