package endpoint_test

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/contracttest/golden"
	"github.com/kbukum/gokit/server/endpoint"
)

// These goldens pin the transport-layer wire shapes shared with rskit. Dynamic
// fields (timestamp, uptime, runtime metrics, build info) are set to fixed
// values so the fixtures assert the field set, key names, and status vocabulary
// deterministically. The health document reuses the healthy/degraded/unhealthy
// vocabulary owned by component.HealthStatus.

func TestHealthResponseGoldenJSON(t *testing.T) {
	t.Parallel()

	resp := endpoint.HealthResponse{
		Status:    string(component.StatusDegraded),
		Service:   "orders",
		Timestamp: "2026-03-05T10:00:00Z",
		Components: []component.Health{
			{Name: "db", Status: component.StatusHealthy},
			{Name: "cache", Status: component.StatusDegraded, Message: "slow"},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal HealthResponse: %v", err)
	}
	golden.AssertJSON(t, data, `{
		"status": "degraded",
		"service": "orders",
		"timestamp": "2026-03-05T10:00:00Z",
		"components": [
			{"name": "db", "status": "healthy"},
			{"name": "cache", "status": "degraded", "message": "slow"}
		]
	}`)
}

func TestReadinessResponseGoldenJSON(t *testing.T) {
	t.Parallel()

	resp := endpoint.ReadinessResponse{
		Status:    "ready",
		Service:   "orders",
		Timestamp: "2026-03-05T10:00:00Z",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal ReadinessResponse: %v", err)
	}
	golden.AssertJSON(t, data, `{
		"status": "ready",
		"service": "orders",
		"timestamp": "2026-03-05T10:00:00Z"
	}`)
}

func TestLivenessResponseGoldenJSON(t *testing.T) {
	t.Parallel()

	resp := endpoint.LivenessResponse{
		Status:    "alive",
		Service:   "orders",
		Timestamp: "2026-03-05T10:00:00Z",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal LivenessResponse: %v", err)
	}
	golden.AssertJSON(t, data, `{
		"status": "alive",
		"service": "orders",
		"timestamp": "2026-03-05T10:00:00Z"
	}`)
}

func TestInfoResponseGoldenJSON(t *testing.T) {
	t.Parallel()

	resp := endpoint.InfoResponse{
		Service:   "orders",
		Version:   "v0.1.0",
		GitCommit: "abc1234",
		GitBranch: "main",
		BuildTime: "2026-03-05T10:00:00Z",
		GoVersion: "go1.25",
		IsRelease: false,
		IsDirty:   false,
		Uptime:    "2h30m15s",
		Timestamp: "2026-03-05T10:00:00Z",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal InfoResponse: %v", err)
	}
	golden.AssertJSON(t, data, `{
		"service": "orders",
		"version": "v0.1.0",
		"git_commit": "abc1234",
		"git_branch": "main",
		"build_time": "2026-03-05T10:00:00Z",
		"go_version": "go1.25",
		"is_release": false,
		"is_dirty": false,
		"uptime": "2h30m15s",
		"timestamp": "2026-03-05T10:00:00Z"
	}`)
}

func TestMetricsResponseGoldenJSON(t *testing.T) {
	t.Parallel()

	resp := endpoint.MetricsResponse{
		Timestamp:  "2026-03-05T10:00:00Z",
		Goroutines: 42,
		Memory: endpoint.MemoryMetrics{
			AllocMB:      24,
			TotalAllocMB: 128,
			SysMB:        64,
			GCRuns:       15,
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal MetricsResponse: %v", err)
	}
	golden.AssertJSON(t, data, `{
		"timestamp": "2026-03-05T10:00:00Z",
		"goroutines": 42,
		"memory": {
			"alloc_mb": 24,
			"total_alloc_mb": 128,
			"sys_mb": 64,
			"gc_runs": 15
		}
	}`)
}
