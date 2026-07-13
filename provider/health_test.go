package provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/kbukum/gokit/provider"
)

func TestHealthStatus_Transitions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status   provider.Status
		expected string
	}{
		{provider.StatusHealthy, "healthy"},
		{provider.StatusDegraded, "degraded"},
		{provider.StatusUnavailable, "unavailable"},
		{provider.Status(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()
			if got := tt.status.String(); got != tt.expected {
				t.Fatalf("Status(%d).String() = %q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

func TestHealthCheck_UnavailableProvider(t *testing.T) {
	t.Parallel()
	p := &healthCheckProvider{
		name: "unhealthy",
		status: provider.HealthStatus{
			Status:  provider.StatusUnavailable,
			Message: "database unreachable",
			Details: map[string]any{"error": "connection refused"},
		},
	}

	health := p.Health(context.Background())
	if health.Status != provider.StatusUnavailable {
		t.Fatalf("expected unavailable, got %v", health.Status)
	}
	if health.Message != "database unreachable" {
		t.Fatalf("expected 'database unreachable', got %q", health.Message)
	}
	if p.IsAvailable(context.Background()) {
		t.Fatal("unavailable provider should return false from IsAvailable")
	}
	if p.IsAvailable(context.Background()) {
		t.Fatal("unavailable provider should return false from IsAvailable")
	}
}

func TestHealthCheck_DegradedProvider(t *testing.T) {
	t.Parallel()
	p := &healthCheckProvider{
		name: "degraded",
		status: provider.HealthStatus{
			Status:  provider.StatusDegraded,
			Message: "high latency",
			Details: map[string]any{"latency_ms": 500},
		},
	}

	health := p.Health(context.Background())
	if health.Status != provider.StatusDegraded {
		t.Fatalf("expected degraded, got %v", health.Status)
	}
	if health.Details["latency_ms"] != 500 {
		t.Fatalf("expected latency_ms=500, got %v", health.Details["latency_ms"])
	}
}

func TestHealthCheck_WithTimeout(t *testing.T) {
	t.Parallel()
	p := &slowHealthProvider{
		name:  "slow-health",
		delay: 200 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	health := p.Health(ctx)
	// Should return unavailable due to timeout
	if health.Status != provider.StatusUnavailable {
		t.Fatalf("expected unavailable due to timeout, got %v", health.Status)
	}
}
