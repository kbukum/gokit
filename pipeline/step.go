package pipeline

import (
	"context"
	"time"
)

// Step represents a named unit of work in an executable pipeline.
type Step[T any] struct {
	ID      string
	Name    string
	Execute func(ctx context.Context, input T) (T, error)
	Skip    func(ctx context.Context, input T) bool // optional: skip if true
}

// StepResult captures the outcome of a step execution.
type StepResult[T any] struct {
	StepID  string        `json:"step_id"`
	Output  T             `json:"output,omitempty"`
	Err     error         `json:"error,omitempty"`
	Elapsed time.Duration `json:"elapsed"`
}
