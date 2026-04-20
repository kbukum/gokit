package pipeline

import (
	"context"
	"fmt"
	"time"
)

// StepStatus describes the status of a step during execution.
type StepStatus string

const (
	StepStarted   StepStatus = "started"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// StepProgress reports progress for a single step in pipeline execution.
type StepProgress[T any] struct {
	StepID string         `json:"step_id"`
	Name   string         `json:"name"`
	Status StepStatus     `json:"status"`
	Result *StepResult[T] `json:"result,omitempty"`
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*executorConfig)

type executorConfig struct {
	// Future extension point for timeout-per-step, retry, etc.
}

// Executor runs a sequence of steps with progress reporting.
type Executor[T any] struct {
	steps []Step[T]
	cfg   executorConfig
}

// NewExecutor creates an Executor for the given steps.
func NewExecutor[T any](steps []Step[T], opts ...ExecutorOption) *Executor[T] {
	cfg := executorConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Executor[T]{
		steps: steps,
		cfg:   cfg,
	}
}

// Execute runs all steps sequentially, calling onProgress for each step's
// lifecycle events. Returns the final output or the first error encountered.
// Supports context cancellation between steps.
func (e *Executor[T]) Execute(ctx context.Context, input T, onProgress func(StepProgress[T])) (T, error) {
	current := input

	for _, step := range e.steps {
		// Check for cancellation before each step.
		if err := ctx.Err(); err != nil {
			return current, fmt.Errorf("pipeline executor: canceled before step %q: %w", step.ID, err)
		}

		// Check skip condition.
		if step.Skip != nil && step.Skip(ctx, current) {
			if onProgress != nil {
				onProgress(StepProgress[T]{
					StepID: step.ID,
					Name:   step.Name,
					Status: StepSkipped,
				})
			}
			continue
		}

		// Report started.
		if onProgress != nil {
			onProgress(StepProgress[T]{
				StepID: step.ID,
				Name:   step.Name,
				Status: StepStarted,
			})
		}

		start := time.Now()
		output, err := step.Execute(ctx, current)
		elapsed := time.Since(start)

		result := &StepResult[T]{
			StepID:  step.ID,
			Output:  output,
			Err:     err,
			Elapsed: elapsed,
		}

		if err != nil {
			if onProgress != nil {
				onProgress(StepProgress[T]{
					StepID: step.ID,
					Name:   step.Name,
					Status: StepFailed,
					Result: result,
				})
			}
			return current, fmt.Errorf("pipeline executor: step %q failed: %w", step.ID, err)
		}

		current = output
		if onProgress != nil {
			onProgress(StepProgress[T]{
				StepID: step.ID,
				Name:   step.Name,
				Status: StepCompleted,
				Result: result,
			})
		}
	}

	return current, nil
}
