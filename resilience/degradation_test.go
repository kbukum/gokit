package resilience

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestNewDegradationManager(t *testing.T) {
	dm := NewDegradationManager()
	if dm == nil {
		t.Fatal("expected non-nil DegradationManager")
	}
	if !dm.IsHealthy() {
		t.Error("new manager with no services should be healthy")
	}
}

func TestDegradationManager_UpdateAndGetService(t *testing.T) {
	dm := NewDegradationManager()

	dm.UpdateService("db", Healthy)
	status := dm.ServiceStatus("db")

	if status.Name != "db" {
		t.Errorf("expected name 'db', got %q", status.Name)
	}
	if status.Health != Healthy {
		t.Errorf("expected Healthy, got %s", status.Health)
	}
	if status.LastCheck.IsZero() {
		t.Error("expected LastCheck to be set")
	}
	if status.LastChange.IsZero() {
		t.Error("expected LastChange to be set")
	}
}

func TestDegradationManager_UpdateServiceWithError(t *testing.T) {
	dm := NewDegradationManager()

	dm.UpdateService("cache", Unhealthy, errors.New("connection refused"))
	status := dm.ServiceStatus("cache")

	if status.Health != Unhealthy {
		t.Errorf("expected Unhealthy, got %s", status.Health)
	}
	if status.Error != "connection refused" {
		t.Errorf("expected error 'connection refused', got %q", status.Error)
	}
}

func TestDegradationManager_LastChangeUpdatesOnHealthChange(t *testing.T) {
	dm := NewDegradationManager()

	dm.UpdateService("api", Healthy)
	first := dm.ServiceStatus("api")

	// Same health → LastChange should not change.
	dm.UpdateService("api", Healthy)
	second := dm.ServiceStatus("api")

	if !second.LastChange.Equal(first.LastChange) {
		t.Error("LastChange should not change when health stays the same")
	}

	// Different health → LastChange should update.
	dm.UpdateService("api", Degraded)
	third := dm.ServiceStatus("api")

	if third.LastChange.Equal(first.LastChange) {
		t.Error("LastChange should update when health changes")
	}
}

func TestDegradationManager_UnknownService(t *testing.T) {
	dm := NewDegradationManager()
	status := dm.ServiceStatus("nonexistent")

	if status.Name != "" {
		t.Errorf("expected empty name for unknown service, got %q", status.Name)
	}
}

func TestDegradationManager_AllStatuses(t *testing.T) {
	dm := NewDegradationManager()

	dm.UpdateService("db", Healthy)
	dm.UpdateService("cache", Degraded)
	dm.UpdateService("api", Unhealthy)

	statuses := dm.AllStatuses()

	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}
	if statuses["db"].Health != Healthy {
		t.Errorf("expected db=Healthy, got %s", statuses["db"].Health)
	}
	if statuses["cache"].Health != Degraded {
		t.Errorf("expected cache=Degraded, got %s", statuses["cache"].Health)
	}
	if statuses["api"].Health != Unhealthy {
		t.Errorf("expected api=Unhealthy, got %s", statuses["api"].Health)
	}
}

func TestDegradationManager_AllStatuses_ReturnsSnapshot(t *testing.T) {
	dm := NewDegradationManager()
	dm.UpdateService("svc", Healthy)

	snapshot := dm.AllStatuses()
	dm.UpdateService("svc", Unhealthy)

	if snapshot["svc"].Health != Healthy {
		t.Error("snapshot should not be affected by subsequent updates")
	}
}

func TestDegradationManager_FeatureFlags(t *testing.T) {
	dm := NewDegradationManager()

	if dm.FeatureEnabled("ai-analysis") {
		t.Error("unknown feature should default to false")
	}

	dm.SetFeature("ai-analysis", true)
	if !dm.FeatureEnabled("ai-analysis") {
		t.Error("expected ai-analysis to be enabled")
	}

	dm.SetFeature("ai-analysis", false)
	if dm.FeatureEnabled("ai-analysis") {
		t.Error("expected ai-analysis to be disabled")
	}
}

func TestDegradationManager_IsHealthy(t *testing.T) {
	dm := NewDegradationManager()

	// No services tracked → healthy.
	if !dm.IsHealthy() {
		t.Error("expected healthy with no services")
	}

	dm.UpdateService("db", Healthy)
	dm.UpdateService("cache", Healthy)
	if !dm.IsHealthy() {
		t.Error("expected healthy when all services are healthy")
	}

	dm.UpdateService("cache", Degraded)
	if dm.IsHealthy() {
		t.Error("expected not healthy when a service is degraded")
	}

	dm.UpdateService("cache", Healthy)
	dm.UpdateService("db", Unhealthy)
	if dm.IsHealthy() {
		t.Error("expected not healthy when a service is unhealthy")
	}
}

func TestDegradationManager_OnCircuitBreakerStateChange(t *testing.T) {
	dm := NewDegradationManager()

	cb := dm.OnCircuitBreakerStateChange("payment-service")

	// Simulate CB state transitions.
	cb("payment-service", StateClosed, StateOpen)
	if dm.ServiceStatus("payment-service").Health != Unhealthy {
		t.Errorf("expected Unhealthy on StateOpen, got %s", dm.ServiceStatus("payment-service").Health)
	}

	cb("payment-service", StateOpen, StateHalfOpen)
	if dm.ServiceStatus("payment-service").Health != Degraded {
		t.Errorf("expected Degraded on StateHalfOpen, got %s", dm.ServiceStatus("payment-service").Health)
	}

	cb("payment-service", StateHalfOpen, StateClosed)
	if dm.ServiceStatus("payment-service").Health != Healthy {
		t.Errorf("expected Healthy on StateClosed, got %s", dm.ServiceStatus("payment-service").Health)
	}
}

func TestDegradationManager_CircuitBreakerIntegration(t *testing.T) {
	dm := NewDegradationManager()

	config := CircuitBreakerConfig{
		Name:          "test-service",
		MaxFailures:   2,
		Timeout:       10,
		OnStateChange: dm.OnCircuitBreakerStateChange("test-service"),
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit breaker.
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error {
			return errors.New("fail")
		})
	}

	if dm.ServiceStatus("test-service").Health != Unhealthy {
		t.Errorf("expected Unhealthy after CB opens, got %s", dm.ServiceStatus("test-service").Health)
	}
}

func TestDegradationManager_HealthEndpoint_Healthy(t *testing.T) {
	dm := NewDegradationManager()
	dm.UpdateService("db", Healthy)
	dm.UpdateService("cache", Healthy)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", http.NoBody)
	dm.HealthEndpoint().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp healthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %q", resp.Status)
	}
	if len(resp.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(resp.Services))
	}
}

func TestDegradationManager_HealthEndpoint_Degraded(t *testing.T) {
	dm := NewDegradationManager()
	dm.UpdateService("db", Healthy)
	dm.UpdateService("cache", Degraded)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", http.NoBody)
	dm.HealthEndpoint().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}

	var resp healthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("expected status 'degraded', got %q", resp.Status)
	}
}

func TestDegradationManager_HealthEndpoint_NoServices(t *testing.T) {
	dm := NewDegradationManager()

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health", http.NoBody)
	dm.HealthEndpoint().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for empty manager, got %d", rr.Code)
	}
}

func TestDegradationManager_ConcurrentAccess(t *testing.T) {
	dm := NewDegradationManager()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			dm.UpdateService("svc", ServiceHealth(i%3))
		}(i)
		go func() {
			defer wg.Done()
			_ = dm.ServiceStatus("svc")
			_ = dm.AllStatuses()
			_ = dm.IsHealthy()
		}()
		go func() {
			defer wg.Done()
			dm.SetFeature("flag", true)
			_ = dm.FeatureEnabled("flag")
		}()
	}
	wg.Wait()
}

func TestServiceHealth_String(t *testing.T) {
	tests := []struct {
		health ServiceHealth
		want   string
	}{
		{Healthy, "healthy"},
		{Degraded, "degraded"},
		{Unhealthy, "unhealthy"},
		{ServiceHealth(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.health.String(); got != tt.want {
			t.Errorf("ServiceHealth(%d).String() = %s, want %s", tt.health, got, tt.want)
		}
	}
}
