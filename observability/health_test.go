package observability

import (
	"fmt"
	"testing"
)

func TestNewServiceHealth(t *testing.T) {
	sh := NewServiceHealth("my-service", "1.0.0")

	if sh.Service != "my-service" {
		t.Errorf("expected Service 'my-service', got %s", sh.Service)
	}
	if sh.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %s", sh.Version)
	}
	if sh.Status != HealthStatusHealthy {
		t.Errorf("expected Status 'healthy', got %s", sh.Status)
	}
}

func TestServiceHealth_AddComponent(t *testing.T) {
	sh := NewServiceHealth("my-service", "1.0.0")

	sh.AddComponent(Health{Name: "db", Status: HealthStatusHealthy})
	if sh.Status != HealthStatusHealthy {
		t.Errorf("expected status 'healthy' after healthy component, got %s", sh.Status)
	}

	sh.AddComponent(Health{Name: "cache", Status: HealthStatusDegraded, Message: "high latency"})
	if sh.Status != HealthStatusDegraded {
		t.Errorf("expected status 'degraded', got %s", sh.Status)
	}

	sh.AddComponent(Health{Name: "queue", Status: HealthStatusUnhealthy, Message: "connection refused"})
	if sh.Status != HealthStatusUnhealthy {
		t.Errorf("expected status 'unhealthy', got %s", sh.Status)
	}

	if len(sh.Components) != 3 {
		t.Errorf("expected 3 components, got %d", len(sh.Components))
	}
}

func TestServiceHealth_DegradedDoesNotOverrideUnhealthy(t *testing.T) {
	sh := NewServiceHealth("svc", "1.0.0")
	sh.AddComponent(Health{Name: "a", Status: HealthStatusUnhealthy})
	sh.AddComponent(Health{Name: "b", Status: HealthStatusDegraded})

	if sh.Status != HealthStatusUnhealthy {
		t.Errorf("expected 'unhealthy' not overridden by 'degraded', got %s", sh.Status)
	}
}

func TestServiceHealthSequentialAddManyComponents(t *testing.T) {
	sh := NewServiceHealth("svc", "1.0.0")

	for i := 0; i < 50; i++ {
		status := HealthStatusHealthy
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
	if sh.Status == HealthStatusHealthy {
		t.Error("expected status to be degraded after adding degraded components")
	}
}

func TestServiceHealthEmptyComponents(t *testing.T) {
	sh := NewServiceHealth("svc", "1.0.0")
	if sh.Status != HealthStatusHealthy {
		t.Errorf("empty service should be healthy, got %q", sh.Status)
	}
	if len(sh.Components) != 0 {
		t.Errorf("expected 0 components, got %d", len(sh.Components))
	}
}

func TestServiceHealthMultipleUnhealthy(t *testing.T) {
	sh := NewServiceHealth("svc", "1.0.0")
	sh.AddComponent(Health{Name: "a", Status: HealthStatusUnhealthy})
	sh.AddComponent(Health{Name: "b", Status: HealthStatusUnhealthy})

	if sh.Status != HealthStatusUnhealthy {
		t.Errorf("expected 'unhealthy', got %q", sh.Status)
	}
}

func TestHealthStatusConstants(t *testing.T) {
	if HealthStatusHealthy != "healthy" {
		t.Errorf("expected 'healthy', got %q", HealthStatusHealthy)
	}
	if HealthStatusUnhealthy != "unhealthy" {
		t.Errorf("expected 'unhealthy', got %q", HealthStatusUnhealthy)
	}
	if HealthStatusDegraded != "degraded" {
		t.Errorf("expected 'degraded', got %q", HealthStatusDegraded)
	}
}

func TestHealthStructFields(t *testing.T) {
	h := Health{
		Name:    "db",
		Status:  HealthStatusHealthy,
		Message: "connected",
		Details: map[string]string{"host": "localhost", "port": "5432"},
	}

	if h.Name != "db" {
		t.Errorf("Name: got %q", h.Name)
	}
	if h.Status != HealthStatusHealthy {
		t.Errorf("Status: got %q", h.Status)
	}
	if h.Message != "connected" {
		t.Errorf("Message: got %q", h.Message)
	}
	if len(h.Details) != 2 {
		t.Errorf("Details length: got %d", len(h.Details))
	}
}
