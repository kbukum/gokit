package chain

import (
	"context"
	stderrors "errors"

	"github.com/kbukum/gokit/errors"
)

// cleanupAction is a deferred cleanup captured for a completed step.
type cleanupAction func(ctx context.Context) error

// chainState threads the current typed output and the cleanups accumulated by
// completed steps through the runner composition.
type chainState[O any] struct {
	output   O
	cleanups []cleanupAction
}

// chainContext carries execution-wide concerns shared by every step.
type chainContext struct {
	progress ChainProgressFn
}

// runner is the composed execution function built by the builder. Each Then
// call wraps the previous runner, transforming the output type.
type runner[I, O any] func(ctx context.Context, input I, cctx chainContext) (chainState[O], error)

// runCleanups executes cleanups in reverse (LIFO) order, aggregating every
// failure so no cleanup is skipped because an earlier one errored.
func runCleanups(ctx context.Context, cleanups []cleanupAction) error {
	var errs []error
	for i := len(cleanups) - 1; i >= 0; i-- {
		if err := cleanups[i](ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return stderrors.Join(errs...)
}

// Chain executes a typed sequence of steps, short-circuiting on the first
// failure. On success it returns the chain's final output type O; on failure it
// returns an error together with the zero value of O.
type Chain[I, O any] struct {
	stepCount int
	runner    runner[I, O]
}

// Execute runs the chain against input, returning the final typed output.
//
// Execution short-circuits on the first failed step and returns its error.
// Cancellation is checked before each step via ctx. When a step fails or the
// chain is canceled, the cleanups registered by already-completed steps run in
// reverse order; any cleanup error is joined onto the returned error. The
// progress callback, when non-nil, receives Running/Completed updates per step.
func (c *Chain[I, O]) Execute(ctx context.Context, input I, progress ChainProgressFn) (O, error) {
	state, err := c.runner(ctx, input, chainContext{progress: progress})
	if err != nil {
		var zero O
		return zero, err
	}
	return state.output, nil
}

// Len returns the number of steps in the chain.
func (c *Chain[I, O]) Len() int { return c.stepCount }

// IsEmpty reports whether the chain has no steps.
func (c *Chain[I, O]) IsEmpty() bool { return c.stepCount == 0 }

// cancelError builds the error returned when the chain is canceled before a
// step runs, joining any cleanup failure that occurred while unwinding.
func cancelError(ctx context.Context, stepID string, cleanupErr error) error {
	err := errors.Canceled("chain").WithCause(ctx.Err()).WithDetail("step", stepID)
	if cleanupErr != nil {
		return stderrors.Join(err, cleanupErr)
	}
	return err
}

// stepError wraps a step failure, preserving the underlying AppError code while
// attaching the failing step id, and joins any cleanup failure. stepErr is
// always non-nil here (callers invoke it only on a failed step), but guard the
// invariant explicitly so a future caller can never trigger a nil dereference.
func stepError(stepID string, stepErr, cleanupErr error) error {
	wrapped := errors.Wrap(stepErr)
	if wrapped == nil {
		if cleanupErr != nil {
			return cleanupErr
		}
		return nil
	}
	out := *wrapped
	out.Details = cloneDetails(wrapped.Details)
	err := out.WithDetail("step", stepID)
	if cleanupErr != nil {
		return stderrors.Join(err, cleanupErr)
	}
	return err
}

// cloneDetails copies a details map so attaching step metadata never mutates a
// shared AppError instance owned by the caller.
func cloneDetails(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src)+1)
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
