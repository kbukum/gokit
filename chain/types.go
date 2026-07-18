package chain

// StepStatus reports the lifecycle state of a chain step as it is emitted through a ChainProgressFn.
// A typed chain short-circuits on the first failure and returns the offending error,
// so only the in-flight states are observed through progress: Running while a step executes
// and Completed once it succeeds. Failure and cancellation are surfaced through the returned error.
type StepStatus string

const (
	// StatusRunning indicates the step has started executing.
	StatusRunning StepStatus = "running"
	// StatusCompleted indicates the step finished successfully.
	StatusCompleted StepStatus = "completed"
)

// StepProgress is a single progress update emitted for a step.
type StepProgress struct {
	// StepIndex is the zero-based position of the step in the chain.
	StepIndex int `json:"step_index"`
	// StepID is the unique identifier of the step.
	StepID string `json:"step_id"`
	// Status is the current lifecycle state of the step.
	Status StepStatus `json:"status"`
	// ProgressPercent is the completion percentage, clamped to 0..=100.
	ProgressPercent uint8 `json:"progress_percent"`
	// Message is an optional human-readable progress message.
	Message string `json:"message,omitempty"`
}

// ChainProgressFn receives chain-level progress updates.
// It must be safe to call synchronously from the executing goroutine and must not block.
type ChainProgressFn func(StepProgress)
