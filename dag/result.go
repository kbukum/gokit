package dag

import "time"

// Node execution status constants.
const (
	StatusCompleted       = "completed"           // Ran successfully
	StatusFailed          = "failed"              // Ran and returned an error
	StatusSkipped         = "skipped"             // Filtered out by schedule/condition
	StatusUnavailable     = "unavailable"         // Node itself is unavailable (optional + not registered)
	StatusDepUnavailable  = "skipped:dep_unavailable" // Skipped because an upstream was unavailable
	StatusDepFailed       = "skipped:dep_failed"      // Skipped because an upstream failed
)

// Result holds the outcome of a graph execution.
type Result struct {
	NodeResults map[string]NodeResult
	Duration    time.Duration
}

// NodeResult holds the outcome of a single node execution.
type NodeResult struct {
	Name     string
	Status   string
	Duration time.Duration
	Output   any
	Error    error
}

// IsTerminal returns true if the node ran (completed or failed).
func (r NodeResult) IsTerminal() bool {
	return r.Status == StatusCompleted || r.Status == StatusFailed
}

// IsSkipped returns true if the node was skipped for any reason.
func (r NodeResult) IsSkipped() bool {
	return r.Status == StatusSkipped || r.Status == StatusDepUnavailable || r.Status == StatusDepFailed
}

// IsSuccess returns true if the node completed successfully.
func (r NodeResult) IsSuccess() bool {
	return r.Status == StatusCompleted
}
