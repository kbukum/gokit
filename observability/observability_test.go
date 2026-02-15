package observability

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
)

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

func TestDefaultMeterConfig(t *testing.T) {
	cfg := DefaultMeterConfig("test-service")

	if cfg.ServiceName != "test-service" {
		t.Errorf("expected ServiceName 'test-service', got %s", cfg.ServiceName)
	}
	if cfg.Interval != 15*time.Second {
		t.Errorf("expected Interval 15s, got %v", cfg.Interval)
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
	metrics.RecordRequestEnd(ctx, "svc", "GET /test", "ok", 100*time.Millisecond)
	metrics.RecordOperation(ctx, "svc", "create", "ok", 50*time.Millisecond)
	metrics.RecordError(ctx, "validation", "handler")
}

func TestNewOperationContext(t *testing.T) {
	oc := NewOperationContext("backend", "create-user", "req-1", "user-1", nil)

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
	oc := NewOperationContext("backend", "create-user", "req-1", "user-1", nil)
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
	oc := NewOperationContext("backend", "create-user", "req-1", "", nil)
	oc.StartTime = time.Now().Add(-50 * time.Millisecond)

	duration := oc.Duration()
	if duration < 45*time.Millisecond || duration > 200*time.Millisecond {
		t.Errorf("expected duration around 50ms, got %v", duration)
	}
}

func TestOperationContext_NilMetrics(t *testing.T) {
	oc := NewOperationContext("backend", "create-user", "req-1", "", nil)
	ctx := context.Background()

	ctx, span := oc.StartSpanForOperation(ctx, "test.op")
	oc.EndOperation(ctx, span, "ok", nil)
}

func TestNewServiceHealth(t *testing.T) {
	sh := NewServiceHealth("my-service", "1.0.0")

	if sh.Service != "my-service" {
		t.Errorf("expected Service 'my-service', got %s", sh.Service)
	}
	if sh.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %s", sh.Version)
	}
	if sh.Status != HealthStatusUp {
		t.Errorf("expected Status 'up', got %s", sh.Status)
	}
}

func TestServiceHealth_AddComponent(t *testing.T) {
	sh := NewServiceHealth("my-service", "1.0.0")

	sh.AddComponent(ComponentHealth{Name: "db", Status: HealthStatusUp})
	if sh.Status != HealthStatusUp {
		t.Errorf("expected status 'up' after healthy component, got %s", sh.Status)
	}

	sh.AddComponent(ComponentHealth{Name: "cache", Status: HealthStatusDegraded, Message: "high latency"})
	if sh.Status != HealthStatusDegraded {
		t.Errorf("expected status 'degraded', got %s", sh.Status)
	}

	sh.AddComponent(ComponentHealth{Name: "queue", Status: HealthStatusDown, Message: "connection refused"})
	if sh.Status != HealthStatusDown {
		t.Errorf("expected status 'down', got %s", sh.Status)
	}

	if len(sh.Components) != 3 {
		t.Errorf("expected 3 components, got %d", len(sh.Components))
	}
}

func TestServiceHealth_DegradedDoesNotOverrideDown(t *testing.T) {
	sh := NewServiceHealth("svc", "1.0.0")
	sh.AddComponent(ComponentHealth{Name: "a", Status: HealthStatusDown})
	sh.AddComponent(ComponentHealth{Name: "b", Status: HealthStatusDegraded})

	if sh.Status != HealthStatusDown {
		t.Errorf("expected 'down' not overridden by 'degraded', got %s", sh.Status)
	}
}
