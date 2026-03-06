package endpoint

import "github.com/kbukum/gokit/component"

// Response types for gokit system endpoints.
// Projects can reference these in OpenAPI annotations, e.g.:
//
//	@Success 200 {object} endpoint.HealthResponse

// HealthResponse is the response from GET /health.
type HealthResponse struct {
	Status     string             `json:"status" example:"healthy"`
	Service    string             `json:"service" example:"my-service"`
	Timestamp  string             `json:"timestamp" example:"2026-03-05T10:00:00Z"`
	Components []component.Health `json:"components"`
}

// InfoResponse is the response from GET /info.
type InfoResponse struct {
	Service   string `json:"service" example:"my-service"`
	Version   string `json:"version" example:"v0.1.0"`
	GitCommit string `json:"git_commit" example:"abc1234"`
	GitBranch string `json:"git_branch" example:"main"`
	BuildTime string `json:"build_time" example:"2026-03-05T10:00:00Z"`
	GoVersion string `json:"go_version" example:"go1.25"`
	IsRelease bool   `json:"is_release" example:"false"`
	IsDirty   bool   `json:"is_dirty" example:"false"`
	Uptime    string `json:"uptime" example:"2h30m15s"`
	Timestamp string `json:"timestamp" example:"2026-03-05T10:00:00Z"`
}

// MetricsResponse is the response from GET /metrics.
type MetricsResponse struct {
	Timestamp  string        `json:"timestamp" example:"2026-03-05T10:00:00Z"`
	Goroutines int           `json:"goroutines" example:"42"`
	Memory     MemoryMetrics `json:"memory"`
}

// MemoryMetrics contains runtime memory statistics.
type MemoryMetrics struct {
	AllocMB      uint64 `json:"alloc_mb" example:"24"`
	TotalAllocMB uint64 `json:"total_alloc_mb" example:"128"`
	SysMB        uint64 `json:"sys_mb" example:"64"`
	GCRuns       uint32 `json:"gc_runs" example:"15"`
}
