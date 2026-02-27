package dag

import "time"

// Result holds the outcome of a graph execution.
type Result struct {
	NodeResults map[string]NodeResult
	Duration    time.Duration
}

// NodeResult holds the outcome of a single node execution.
type NodeResult struct {
	Name     string
	Status   string // "completed" | "skipped" | "failed"
	Duration time.Duration
	Output   any
	Error    error
}
