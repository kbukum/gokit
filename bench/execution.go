package bench

// ExecutionPlan captures the per-run execution settings used to evaluate benchmark branches.
type ExecutionPlan struct {
	Concurrency int
}

// NewExecutionPlan creates an execution plan and normalizes concurrency to at least one worker.
func NewExecutionPlan(concurrency int) ExecutionPlan {
	if concurrency < 1 {
		concurrency = 1
	}
	return ExecutionPlan{Concurrency: concurrency}
}
