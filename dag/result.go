package dag

import (
	"time"

	"github.com/kbukum/gokit/dag/status"
)

// Result holds the outcome of a graph execution.
type Result struct {
	NodeResults map[string]NodeResult
	Duration    time.Duration
}

// NodeResult holds the outcome of a single node execution.
type NodeResult struct {
	Name     string
	Status   status.Status
	Duration time.Duration
	Output   any
	Error    error
}

// IsTerminal returns true if the node ran (completed or failed).
func (r NodeResult) IsTerminal() bool {
	return r.Status.IsTerminal()
}

// IsSkipped returns true if the node was skipped for any reason.
func (r NodeResult) IsSkipped() bool {
	return r.Status.IsSkipped()
}

// IsSuccess returns true if the node completed successfully.
func (r NodeResult) IsSuccess() bool {
	return r.Status.IsSuccess()
}
