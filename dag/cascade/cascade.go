package cascade

// CascadeBuilder constructs a staged execution pipeline where each stage is a sub-DAG with its own configuration.
// Stages execute sequentially, with an advance condition between each pair of stages.
type CascadeBuilder[I, O any] struct {
	stages      []*stageSpec[I, O]
	finalStage  *stageSpec[I, O]
	onFailure   StageFailurePolicy
	mergeFunc   func(accumulated, stageResult O) O
	orderFunc   OrderStrategy
	maxParallel int
}

// StageFunc is the function signature for stage builder callbacks.
type StageFunc[I, O any] func(b *StageBuilder[I, O], input I)

// StageFailurePolicy defines behavior when an entire stage fails.
type StageFailurePolicy int

const (
	// StageFailureAbort stops the cascade on stage failure.
	StageFailureAbort StageFailurePolicy = iota
	// StageFailureSkipToFinal skips to the final stage with accumulated results.
	StageFailureSkipToFinal
	// StageFailureContinue proceeds to the next stage regardless.
	StageFailureContinue
)

// SkipToFinal returns a policy that skips to the final stage on failure.
func SkipToFinal() StageFailurePolicy { return StageFailureSkipToFinal }

// Abort returns a policy that aborts the cascade on stage failure.
func Abort() StageFailurePolicy { return StageFailureAbort }

// ContinueOnFailure returns a policy that continues to the next stage on failure.
func ContinueOnFailure() StageFailurePolicy { return StageFailureContinue }

// NewCascade creates a new cascade builder.
func NewCascade[I, O any]() *CascadeBuilder[I, O] {
	return &CascadeBuilder[I, O]{
		onFailure: StageFailureAbort,
	}
}

// Stage adds a named stage to the cascade. The builder function receives the input
// and can conditionally add nodes based on it.
func (c *CascadeBuilder[I, O]) Stage(name string, fn StageFunc[I, O]) *CascadeBuilder[I, O] {
	c.stages = append(c.stages, &stageSpec[I, O]{
		name:    name,
		buildFn: fn,
	})
	return c
}

// FinalStage sets the final stage that always executes with all accumulated results,
// regardless of early exit.
func (c *CascadeBuilder[I, O]) FinalStage(name string, fn StageFunc[I, O]) *CascadeBuilder[I, O] {
	c.finalStage = &stageSpec[I, O]{
		name:    name,
		buildFn: fn,
	}
	return c
}

// OnStageFailure sets the policy for when an entire stage fails.
func (c *CascadeBuilder[I, O]) OnStageFailure(policy StageFailurePolicy) *CascadeBuilder[I, O] {
	c.onFailure = policy
	return c
}

// MergeStrategy sets how results from each stage are accumulated.
func (c *CascadeBuilder[I, O]) MergeStrategy(fn func(accumulated, stageResult O) O) *CascadeBuilder[I, O] {
	c.mergeFunc = fn
	return c
}

// OrderNodesBy sets the ordering strategy for parallel nodes within a stage.
func (c *CascadeBuilder[I, O]) OrderNodesBy(strategy OrderStrategy) *CascadeBuilder[I, O] {
	c.orderFunc = strategy
	return c
}

// MaxConcurrency limits parallel node execution within each stage.
func (c *CascadeBuilder[I, O]) MaxConcurrency(n int) *CascadeBuilder[I, O] {
	c.maxParallel = n
	return c
}

// Build creates an executable Cascade.
func (c *CascadeBuilder[I, O]) Build() *Cascade[I, O] {
	return &Cascade[I, O]{
		stages:      c.stages,
		finalStage:  c.finalStage,
		onFailure:   c.onFailure,
		mergeFunc:   c.mergeFunc,
		orderFunc:   c.orderFunc,
		maxParallel: c.maxParallel,
	}
}

// stageSpec holds the definition for a single stage.
type stageSpec[I, O any] struct {
	name    string
	buildFn StageFunc[I, O]
}

// Cascade is an executable staged pipeline.
type Cascade[I, O any] struct {
	stages      []*stageSpec[I, O]
	finalStage  *stageSpec[I, O]
	onFailure   StageFailurePolicy
	mergeFunc   func(accumulated, stageResult O) O
	orderFunc   OrderStrategy
	maxParallel int
}
