package chain

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Config controls chain execution behavior.
type Config struct {
	// CleanupOnFailure runs Cleanup on completed steps in reverse order when
	// a later step fails. Default: true.
	CleanupOnFailure bool
	// StopOnFailure skips remaining steps after the first failure.
	// Default: true.
	StopOnFailure bool
}

// DefaultConfig returns the default chain configuration.
func DefaultConfig() Config {
	return Config{
		CleanupOnFailure: true,
		StopOnFailure:    true,
	}
}

// Executor runs a sequence of operations, feeding each output as input to
// the next step.
type Executor struct {
	operations []Operation
	config     Config
}

// NewExecutor creates an executor from the given operations.
func NewExecutor(ops []Operation) *Executor {
	return &Executor{
		operations: ops,
		config:     DefaultConfig(),
	}
}

// WithConfig overrides the default configuration.
func (e *Executor) WithConfig(cfg Config) *Executor {
	e.config = cfg
	return e
}

// Execute runs all operations sequentially.
//
// The progress callback (if non-nil) receives per-step progress updates.
// Cancellation is checked at step boundaries via ctx.
func (e *Executor) Execute(ctx context.Context, input any, progress ChainProgressFn) (*ChainResult, error) {
	chainStart := time.Now()
	totalSteps := len(e.operations)
	results := make([]StepResult, 0, totalSteps)
	currentInput := input
	failed := false

	for i, op := range e.operations {
		// Check cancellation before starting each step
		if err := ctx.Err(); err != nil {
			for _, remaining := range e.operations[i:] {
				results = append(results, StepResult{
					StepID: remaining.ID(),
					Status: StatusCancelled,
					Error:  "chain canceled",
				})
			}
			break
		}

		// Skip remaining if a previous step failed and StopOnFailure is true
		if failed && e.config.StopOnFailure {
			results = append(results, StepResult{
				StepID: op.ID(),
				Status: StatusSkipped,
			})
			continue
		}

		stepID := op.ID()
		stepStart := time.Now()

		// Emit "running" progress
		if progress != nil {
			progress(StepProgress{
				StepIndex:       i,
				StepID:          stepID,
				Status:          StatusRunning,
				ProgressPercent: 0,
			})
		}

		// Create per-step progress callback that wraps chain-level callback
		var stepProgress ProgressFn
		if progress != nil {
			stepProgress = func(pct uint8, msg string) {
				progress(StepProgress{
					StepIndex:       i,
					StepID:          stepID,
					Status:          StatusRunning,
					ProgressPercent: pct,
					Message:         msg,
				})
			}
		} else {
			stepProgress = func(_ uint8, _ string) {}
		}

		slog.Debug("executing chain step",
			"step", stepID,
			"index", i,
			"total_steps", totalSteps,
		)

		output, err := op.Execute(ctx, currentInput, stepProgress)
		duration := time.Since(stepStart)

		if err != nil {
			slog.Error("chain step failed",
				"step", stepID,
				"error", err,
			)

			if progress != nil {
				progress(StepProgress{
					StepIndex:       i,
					StepID:          stepID,
					Status:          StatusFailed,
					ProgressPercent: 0,
					Message:         err.Error(),
				})
			}

			results = append(results, StepResult{
				StepID:   stepID,
				Status:   StatusFailed,
				Duration: duration,
				Error:    err.Error(),
			})
			failed = true
		} else {
			if progress != nil {
				progress(StepProgress{
					StepIndex:       i,
					StepID:          stepID,
					Status:          StatusCompleted,
					ProgressPercent: 100,
				})
			}

			currentInput = output
			results = append(results, StepResult{
				StepID:   stepID,
				Status:   StatusCompleted,
				Duration: duration,
				Output:   output,
			})
		}
	}

	// Cleanup on failure: call Cleanup on completed steps in reverse order
	allCompleted := !failed && len(results) == totalSteps
	for _, r := range results {
		if r.Status != StatusCompleted {
			allCompleted = false
			break
		}
	}

	if !allCompleted && e.config.CleanupOnFailure {
		slog.Warn("chain failed, cleaning up completed steps")
		for j := len(results) - 1; j >= 0; j-- {
			if results[j].Status != StatusCompleted {
				continue
			}
			for _, op := range e.operations {
				if op.ID() == results[j].StepID {
					if err := op.Cleanup(ctx, results[j].Output); err != nil {
						slog.Error("cleanup failed",
							"step", results[j].StepID,
							"error", err,
						)
					}
					break
				}
			}
		}
	}

	var finalOutput any
	if allCompleted && len(results) > 0 {
		finalOutput = results[len(results)-1].Output
	}

	return &ChainResult{
		Steps:         results,
		TotalDuration: time.Since(chainStart),
		FinalOutput:   finalOutput,
		Success:       allCompleted,
	}, nil
}

// String returns a summary of the executor configuration.
func (e *Executor) String() string {
	return fmt.Sprintf("ChainExecutor(%d steps, stop_on_failure=%v, cleanup=%v)",
		len(e.operations), e.config.StopOnFailure, e.config.CleanupOnFailure)
}
