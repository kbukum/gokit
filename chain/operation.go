package chain

import "context"

// ProgressFn is the per-step progress callback.
// Receives a percentage (0–100) and an optional human-readable message.
type ProgressFn func(percent uint8, message string)

// Operation is a single step in a sequential chain.
//
// Each operation receives the output of the previous step (or the initial
// input) as a JSON-like any value and produces an output for the next step.
type Operation interface {
	// ID returns a unique identifier for this operation.
	ID() string
	// Name returns a human-readable name (may equal ID).
	Name() string
	// Execute runs the operation.
	//   - ctx:      cancellation and deadline propagation
	//   - input:    output from the previous step (or chain input for step 0)
	//   - progress: callback for reporting completion (0–100) + optional message
	Execute(ctx context.Context, input any, progress ProgressFn) (any, error)
	// Cleanup is called when the chain fails after this operation completed.
	// Used to delete intermediate files, release resources, etc.
	// The default implementation is a no-op.
	Cleanup(ctx context.Context, output any) error
}

// BaseOperation provides default implementations for optional Operation methods.
// Embed it in your operation struct to only override what you need:
//
//	type MyOp struct {
//	    chain.BaseOperation
//	}
type BaseOperation struct{}

// Name returns an empty string; override in your struct.
func (BaseOperation) Name() string { return "" }

// Cleanup is a no-op by default.
func (BaseOperation) Cleanup(_ context.Context, _ any) error { return nil }
