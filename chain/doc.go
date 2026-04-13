// Package chain provides a sequential chain execution pattern.
//
// A chain runs a sequence of operations where each step receives the output
// of the previous step.  Supports per-step progress reporting, cancellation
// at step boundaries, and automatic cleanup of completed steps when a later
// step fails.
//
// Mirrors rskit-chain (Rust) and pykit-chain (Python).
//
//	chain := chain.NewBuilder().
//	    Step(myFirstOp).
//	    Step(mySecondOp).
//	    Build()
//
//	result, err := chain.Execute(ctx, input, nil)
package chain
