package resilience

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
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

	// Different health → LastChange should update. On low-resolution
	// clocks (notably Windows, ~16ms granularity) consecutive time.Now()
	// calls can return the same instant, so allow the clock to advance
	// before the change so the LastChange comparison is meaningful.
	time.Sleep(20 * time.Millisecond)
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

func TestDegradation_FeatureDependsOnServiceHealth(t *testing.T) {
	t.Parallel()
	dm := NewDegradationManager()

	dm.UpdateService("payment-api", Healthy)
	dm.SetFeature("checkout", true)

	if !dm.FeatureEnabled("checkout") {
		t.Error("checkout should be enabled when payment-api is healthy")
	}

	// Simulate: when payment-api becomes unhealthy, disable checkout.
	dm.UpdateService("payment-api", Unhealthy)
	if dm.ServiceStatus("payment-api").Health != Unhealthy {
		t.Error("payment-api should be unhealthy")
	}

	// An application would check health + feature. Verify the pattern works.
	status := dm.ServiceStatus("payment-api")
	if status.Health == Unhealthy {
		dm.SetFeature("checkout", false)
	}
	if dm.FeatureEnabled("checkout") {
		t.Error("checkout should be disabled when payment-api is unhealthy")
	}
}

func TestDegradation_OnCBStateChangeWithRealCircuitBreaker(t *testing.T) {
	dm := NewDegradationManager()

	config := CircuitBreakerConfig{
		Name:             "auth",
		MaxFailures:      2,
		Timeout:          15 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		OnStateChange:    dm.OnCircuitBreakerStateChange("auth"),
	}
	cb := NewCircuitBreaker(config)

	// Closed → Open
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}
	if dm.ServiceStatus("auth").Health != Unhealthy {
		t.Errorf("expected Unhealthy, got %s", dm.ServiceStatus("auth").Health)
	}

	// Open → HalfOpen
	time.Sleep(20 * time.Millisecond)
	_ = cb.State() // triggers transition

	if dm.ServiceStatus("auth").Health != Degraded {
		t.Errorf("expected Degraded in half-open, got %s", dm.ServiceStatus("auth").Health)
	}

	// HalfOpen → Closed
	_ = cb.Execute(func() error { return nil })
	if dm.ServiceStatus("auth").Health != Healthy {
		t.Errorf("expected Healthy after close, got %s", dm.ServiceStatus("auth").Health)
	}
}

func TestDegradation_HealthEndpointJSONFormat(t *testing.T) {
	t.Parallel()
	dm := NewDegradationManager()
	dm.UpdateService("db", Healthy)
	dm.UpdateService("cache", Degraded, errors.New("timeout"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	dm.HealthEndpoint().ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp healthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("expected status 'degraded', got %q", resp.Status)
	}
	if len(resp.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(resp.Services))
	}
	if resp.Services["cache"].Error != "timeout" {
		t.Errorf("expected cache error 'timeout', got %q", resp.Services["cache"].Error)
	}
}

func TestDegradation_HealthEndpoint503WhenUnhealthy(t *testing.T) {
	t.Parallel()
	dm := NewDegradationManager()
	dm.UpdateService("db", Healthy)
	dm.UpdateService("api", Unhealthy)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	dm.HealthEndpoint().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestDegradation_ConcurrentServiceHealthUpdates(t *testing.T) {
	t.Parallel()
	dm := NewDegradationManager()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc := fmt.Sprintf("svc-%d", n%5)
			h := ServiceHealth(n % 3)
			dm.UpdateService(svc, h)
			_ = dm.ServiceStatus(svc)
			_ = dm.AllStatuses()
			_ = dm.IsHealthy()
		}(i)
	}
	wg.Wait()

	statuses := dm.AllStatuses()
	if len(statuses) > 5 {
		t.Errorf("expected at most 5 services, got %d", len(statuses))
	}
}

func TestDegradation_FeatureReEnableAfterRecovery(t *testing.T) {
	dm := NewDegradationManager()

	config := CircuitBreakerConfig{
		Name:             "search",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		OnStateChange:    dm.OnCircuitBreakerStateChange("search"),
	}
	cb := NewCircuitBreaker(config)
	dm.SetFeature("advanced-search", true)

	// Trip CB → service becomes unhealthy → disable feature.
	_ = cb.Execute(func() error { return errors.New("fail") })
	if dm.ServiceStatus("search").Health != Unhealthy {
		t.Fatal("search should be unhealthy")
	}
	dm.SetFeature("advanced-search", false)

	// Wait for half-open → recover.
	time.Sleep(15 * time.Millisecond)
	_ = cb.Execute(func() error { return nil }) // half-open → closed

	if dm.ServiceStatus("search").Health != Healthy {
		t.Fatalf("search should be healthy after recovery, got %s", dm.ServiceStatus("search").Health)
	}

	// Re-enable feature.
	dm.SetFeature("advanced-search", true)
	if !dm.FeatureEnabled("advanced-search") {
		t.Error("advanced-search should be re-enabled after recovery")
	}
}
