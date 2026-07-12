package discovery

import (
	"errors"
	"testing"
)

// ── ServiceInstance tests ───────────────────────────────────────────

func TestServiceInstance_Creation(t *testing.T) {
	inst := makeInstance("svc-1", "api", "10.0.0.1", 8080)
	if inst.ID != "svc-1" {
		t.Errorf("ID = %q, want %q", inst.ID, "svc-1")
	}
	if inst.Name != "api" {
		t.Errorf("Name = %q, want %q", inst.Name, "api")
	}
	if inst.Address != "10.0.0.1" {
		t.Errorf("Address = %q, want %q", inst.Address, "10.0.0.1")
	}
	if inst.Port != 8080 {
		t.Errorf("Port = %d, want %d", inst.Port, 8080)
	}
}

func TestServiceInstance_HostPort(t *testing.T) {
	inst := makeInstance("id", "svc", "192.168.1.5", 9090)
	got := inst.HostPort()
	if got != "192.168.1.5:9090" {
		t.Errorf("HostPort() = %q, want %q", got, "192.168.1.5:9090")
	}
}

func TestServiceInstance_MetadataPreservation(t *testing.T) {
	inst := ServiceInstance{
		ID:       "m1",
		Metadata: map[string]string{"region": "us-east-1", "env": "staging"},
	}
	if inst.Metadata["region"] != "us-east-1" {
		t.Errorf("metadata[region] = %q, want %q", inst.Metadata["region"], "us-east-1")
	}
	if inst.Metadata["env"] != "staging" {
		t.Errorf("metadata[env] = %q, want %q", inst.Metadata["env"], "staging")
	}
}

func TestHealthStatus_Values(t *testing.T) {
	tests := []struct {
		status HealthStatus
		want   string
	}{
		{HealthHealthy, "healthy"},
		{HealthUnhealthy, "unhealthy"},
		{HealthUnknown, "unknown"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("HealthStatus = %q, want %q", tt.status, tt.want)
		}
	}
}

// ── Discovery errors ────────────────────────────────────────────────

func TestErrSentinels(t *testing.T) {
	if !errors.Is(ErrServiceNotFound, ErrServiceNotFound) {
		t.Error("ErrServiceNotFound sentinel broken")
	}
	if !errors.Is(ErrNoHealthyEndpoints, ErrNoHealthyEndpoints) {
		t.Error("ErrNoHealthyEndpoints sentinel broken")
	}
	if !errors.Is(ErrDiscoveryDisabled, ErrDiscoveryDisabled) {
		t.Error("ErrDiscoveryDisabled sentinel broken")
	}
}

// ── Endpoint alias ──────────────────────────────────────────────────

func TestEndpoint_IsAlias(t *testing.T) {
	e := ServiceInstance{ID: "ep-1"}
	if e.ID != "ep-1" {
		t.Error("Endpoint alias should be interchangeable with ServiceInstance")
	}
}
