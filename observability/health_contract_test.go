package observability_test

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/contracttest/golden"
	"github.com/kbukum/gokit/observability"
)

// The ServiceHealth wire shape is a cross-kit contract shared with rskit and
// reused by server/discovery. The status vocabulary is exactly
// healthy/degraded/unhealthy — never up/down. These goldens pin the same
// examples as rskit's service-health fixtures.

func TestServiceHealthHealthyGoldenJSON(t *testing.T) {
	t.Parallel()

	sh := observability.NewServiceHealth("orders", "1.2.3")
	data, err := json.Marshal(sh)
	if err != nil {
		t.Fatalf("marshal ServiceHealth: %v", err)
	}
	golden.AssertJSON(t, data, `{"service":"orders","status":"healthy","version":"1.2.3"}`)
}

func TestServiceHealthDegradedGoldenJSON(t *testing.T) {
	t.Parallel()

	sh := observability.NewServiceHealth("orders", "1.2.3")
	sh.AddComponent(observability.Health{Name: "cache", Status: observability.HealthStatusDegraded, Message: "high latency"})
	data, err := json.Marshal(sh)
	if err != nil {
		t.Fatalf("marshal ServiceHealth: %v", err)
	}
	golden.AssertJSON(t, data, `{
		"service": "orders",
		"status": "degraded",
		"version": "1.2.3",
		"components": [
			{"name": "cache", "status": "degraded", "message": "high latency"}
		]
	}`)
}

func TestServiceHealthUnhealthyGoldenJSON(t *testing.T) {
	t.Parallel()

	sh := observability.NewServiceHealth("orders", "1.2.3")
	sh.AddComponent(observability.Health{Name: "db", Status: observability.HealthStatusUnhealthy, Message: "connection refused"})
	data, err := json.Marshal(sh)
	if err != nil {
		t.Fatalf("marshal ServiceHealth: %v", err)
	}
	golden.AssertJSON(t, data, `{
		"service": "orders",
		"status": "unhealthy",
		"version": "1.2.3",
		"components": [
			{"name": "db", "status": "unhealthy", "message": "connection refused"}
		]
	}`)
}
