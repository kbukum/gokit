// Package chain provides typed, sequential chain execution.
//
// A chain is a statically typed sequence of steps where each step consumes the previous step's output
// and produces the next step's input type. Execution short-circuits on the first error,
// checks cancellation between steps,
// and runs the cleanup actions registered by already-completed steps (in reverse order) when a later step fails
// or the chain is canceled.
//
// Because Go methods cannot introduce new type parameters, steps are appended with the package-level Then function rather than a fluent method. For chains longer than a couple of steps, bind each intermediate builder so the code reads in execution order and mirrors the package-level composition style used by stream:
//
//	parse := chain.StepFunc("parse", func(_ chain.StepContext, in string) (int, error) {
//		return strconv.Atoi(in)
//	})
//	double := chain.StepFunc("double", func(_ chain.StepContext, n int) (int, error) {
//		return n * 2, nil
//	})
//
//	start := chain.New[string]()
//	parsed := chain.Then(start, parse)
//	doubled := chain.Then(parsed, double)
//	c := doubled.Build()
//
//	out, err := c.Execute(ctx, "21", nil) // out == 42
//
// Mirrors rskit-chain (Rust) and pykit-chain (Python):
// the same typed Step / builder / cleanup-on-failure semantics, expressed idiomatically in Go.
package chain
