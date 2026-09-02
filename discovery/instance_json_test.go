package discovery_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kbukum/gokit/contracttest/golden"
	"github.com/kbukum/gokit/discovery"
)

// The ServiceInstance wire shape is gokit's stable discovery contract:
// snake_case fields, tri-state health (unknown/healthy/unhealthy), an explicit
// protocol and last_seen, and an omitted weight that decodes to 1.

func TestServiceInstanceGoldenJSON(t *testing.T) {
	t.Parallel()

	inst := discovery.ServiceInstance{
		ID:       "orders-10.0.0.5-8080",
		Name:     "orders",
		Address:  "10.0.0.5",
		Port:     8080,
		Protocol: "http",
		Tags:     []string{"canary", "us-east-1"},
		Metadata: map[string]string{"zone": "a"},
		Health:   discovery.HealthHealthy,
		Weight:   5,
		LastSeen: time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(inst)
	if err != nil {
		t.Fatalf("marshal ServiceInstance: %v", err)
	}
	golden.AssertJSON(t, data, `{
		"id": "orders-10.0.0.5-8080",
		"name": "orders",
		"address": "10.0.0.5",
		"port": 8080,
		"protocol": "http",
		"tags": ["canary", "us-east-1"],
		"metadata": {"zone": "a"},
		"health": "healthy",
		"weight": 5,
		"last_seen": "2026-03-05T10:00:00Z"
	}`)
}

func TestServiceInstanceOmittedWeightDefaultsToOne(t *testing.T) {
	t.Parallel()

	var inst discovery.ServiceInstance
	if err := json.Unmarshal([]byte(`{
		"id": "a",
		"name": "svc",
		"address": "10.0.0.1",
		"port": 9090,
		"protocol": "grpc",
		"health": "unknown"
	}`), &inst); err != nil {
		t.Fatalf("unmarshal ServiceInstance: %v", err)
	}
	if inst.Weight != 1 {
		t.Errorf("omitted weight: got %d want 1", inst.Weight)
	}
	if inst.Health != discovery.HealthUnknown {
		t.Errorf("health: got %q want unknown", inst.Health)
	}
}

func TestServiceInstanceExplicitZeroWeightPreserved(t *testing.T) {
	t.Parallel()

	var inst discovery.ServiceInstance
	if err := json.Unmarshal([]byte(`{"id":"a","name":"svc","weight":0}`), &inst); err != nil {
		t.Fatalf("unmarshal ServiceInstance: %v", err)
	}
	if inst.Weight != 0 {
		t.Errorf("explicit zero weight: got %d want 0", inst.Weight)
	}
}

func TestServiceInstanceZeroHealthMarshalsUnknown(t *testing.T) {
	t.Parallel()

	// A zero-value Health must serialize as the tri-state "unknown", never "".
	data, err := json.Marshal(discovery.ServiceInstance{ID: "a", Name: "svc"})
	if err != nil {
		t.Fatalf("marshal ServiceInstance: %v", err)
	}
	var decoded struct {
		Health string `json:"health"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal probe: %v", err)
	}
	if decoded.Health != "unknown" {
		t.Errorf("zero health: got %q want %q", decoded.Health, "unknown")
	}
}

func TestServiceInstanceOmittedHealthDefaultsToUnknown(t *testing.T) {
	t.Parallel()

	var inst discovery.ServiceInstance
	if err := json.Unmarshal([]byte(`{"id":"a","name":"svc"}`), &inst); err != nil {
		t.Fatalf("unmarshal ServiceInstance: %v", err)
	}
	if inst.Health != discovery.HealthUnknown {
		t.Errorf("omitted health: got %q want unknown", inst.Health)
	}
}

func TestServiceInstanceUnmarshalRejectsInvalidHealth(t *testing.T) {
	t.Parallel()

	var inst discovery.ServiceInstance
	err := json.Unmarshal([]byte(`{"id":"x","health":"degraded"}`), &inst)
	if err == nil {
		t.Fatal("expected error decoding out-of-range health, got nil")
	}
}

func TestServiceInstanceUnmarshalDefaultsHealthUnknown(t *testing.T) {
	t.Parallel()

	var inst discovery.ServiceInstance
	if err := json.Unmarshal([]byte(`{"id":"x"}`), &inst); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if inst.Health != discovery.HealthUnknown {
		t.Fatalf("omitted health: got %q want unknown", inst.Health)
	}
	if inst.Weight != 1 {
		t.Fatalf("omitted weight: got %d want 1", inst.Weight)
	}
}

func TestHealthStatusMarshalRejectsInvalid(t *testing.T) {
	t.Parallel()

	if _, err := json.Marshal(discovery.HealthStatus("degraded")); err == nil {
		t.Fatal("expected error marshaling out-of-range health, got nil")
	}
	got, err := json.Marshal(discovery.HealthStatus(""))
	if err != nil {
		t.Fatalf("marshal zero health: %v", err)
	}
	if string(got) != `"unknown"` {
		t.Fatalf("zero health: got %s want \"unknown\"", got)
	}
}
