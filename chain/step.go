package chain

import "context"

// StepContext is the per-step execution context.
// It carries cancellation (through the underlying context.Context)
// and a callback for reporting step-local progress. It is passed by value;
// the zero value is not useful — obtain one from the executor.
type StepContext struct {
	ctx      context.Context
	progress func(percent uint8, message string)
}

// newStepContext builds a StepContext bound to ctx with an optional step-local progress reporter.
func newStepContext(ctx context.Context, progress func(percent uint8, message string)) StepContext {
	return StepContext{ctx: ctx, progress: progress}
}

// Context returns the underlying context.Context shared by the chain execution.
func (c StepContext) Context() context.Context { return c.ctx }

// Err reports why the context was canceled, or nil if it is still live.
func (c StepContext) Err() error { return c.ctx.Err() }

// Progress reports step-local progress. percent is clamped to 0..=100 and message may be empty.
// It is a no-op when no progress reporter is attached.
func (c StepContext) Progress(percent uint8, message string) {
	if c.progress == nil {
		return
	}
	if percent > 100 {
		percent = 100
	}
	c.progress(percent, message)
}

// StepFn executes a single typed step.
// It receives the previous step's output (or the chain input for the first step)
// and returns this step's output.
type StepFn[I, O any] func(sctx StepContext, input I) (O, error)

// CleanupFn releases resources produced by a completed step. It runs only when a later step fails
// or the chain is canceled, in reverse (LIFO) order.
type CleanupFn func(ctx context.Context) error

// Step is a typed operation in a sequential chain: it consumes an I
// and produces an O. Steps are values; construct them with NewStep or StepFunc
// and register cleanup with WithCleanup.
type Step[I, O any] struct {
	id      string
	name    string
	execute StepFn[I, O]
	cleanup CleanupFn
}

// NewStep creates a typed step with an explicit id and human-readable name.
func NewStep[I, O any](id, name string, execute StepFn[I, O]) Step[I, O] {
	return Step[I, O]{id: id, name: name, execute: execute}
}

// StepFunc creates a typed step using id as both identifier and display name.
func StepFunc[I, O any](id string, execute StepFn[I, O]) Step[I, O] {
	return NewStep(id, id, execute)
}

// WithCleanup registers a cleanup action that runs only if a later step fails
// or the chain is canceled after this step has completed. It returns a copy of the step;
// the receiver is not modified.
func (s Step[I, O]) WithCleanup(cleanup CleanupFn) Step[I, O] {
	s.cleanup = cleanup
	return s
}

// ID returns the unique step identifier.
func (s Step[I, O]) ID() string { return s.id }

// Name returns the human-readable step name.
func (s Step[I, O]) Name() string { return s.name }
