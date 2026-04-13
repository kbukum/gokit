package chain

import (
	"encoding/json"
	"time"
)

// StepStatus represents the current state of a chain step.
type StepStatus string

const (
	StatusPending   StepStatus = "pending"
	StatusRunning   StepStatus = "running"
	StatusCompleted StepStatus = "completed"
	StatusFailed    StepStatus = "failed"
	StatusSkipped   StepStatus = "skipped"
	StatusCancelled StepStatus = "cancelled"
)

// StepProgress is a progress update for a single step.
type StepProgress struct {
	StepIndex       int        `json:"step_index"`
	StepID          string     `json:"step_id"`
	Status          StepStatus `json:"status"`
	ProgressPercent uint8      `json:"progress_percent"`
	Message         string     `json:"message,omitempty"`
}

// ChainProgressFn is the chain-level progress callback.
type ChainProgressFn func(StepProgress)

// StepResult holds the outcome of a single step execution.
type StepResult struct {
	StepID   string        `json:"step_id"`
	Status   StepStatus    `json:"status"`
	Duration time.Duration `json:"duration"`
	Output   any           `json:"output"`
	Error    string        `json:"error,omitempty"`
}

// ChainResult holds the overall chain execution result.
type ChainResult struct {
	Steps         []StepResult  `json:"steps"`
	TotalDuration time.Duration `json:"total_duration"`
	FinalOutput   any           `json:"final_output,omitempty"`
	Success       bool          `json:"success"`
}

// CompletedSteps returns the number of steps that finished successfully.
func (r *ChainResult) CompletedSteps() int {
	n := 0
	for _, s := range r.Steps {
		if s.Status == StatusCompleted {
			n++
		}
	}
	return n
}

// FailedStep returns the first failed step, or nil if none failed.
func (r *ChainResult) FailedStep() *StepResult {
	for i := range r.Steps {
		if r.Steps[i].Status == StatusFailed {
			return &r.Steps[i]
		}
	}
	return nil
}

// MarshalJSON implements custom JSON serialization for ChainResult.
func (r *ChainResult) MarshalJSON() ([]byte, error) {
	type Alias ChainResult
	return json.Marshal(&struct {
		*Alias
		TotalDuration string `json:"total_duration"`
	}{
		Alias:         (*Alias)(r),
		TotalDuration: r.TotalDuration.String(),
	})
}
