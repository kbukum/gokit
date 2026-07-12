package chain

import "context"

// Builder incrementally composes a typed chain. It threads two type
// parameters: I is the chain input type and O is the output type produced so
// far. Add steps with the package-level Then function (Go methods cannot
// introduce new type parameters), then call Build.
type Builder[I, O any] struct {
	stepCount int
	runner    runner[I, O]
}

// New creates an empty builder whose input and current output types are both T.
func New[T any]() *Builder[T, T] {
	return &Builder[T, T]{
		runner: func(_ context.Context, input T, _ chainContext) (chainState[T], error) {
			return chainState[T]{output: input}, nil
		},
	}
}

// Then appends step to the builder, transforming the current output type from
// M to N. It returns a new builder; the input builder is left unchanged.
func Then[I, M, N any](b *Builder[I, M], step Step[M, N]) *Builder[I, N] {
	previous := b.runner
	stepIndex := b.stepCount
	next := func(ctx context.Context, input I, cctx chainContext) (chainState[N], error) {
		state, err := previous(ctx, input, cctx)
		if err != nil {
			return chainState[N]{}, err
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return chainState[N]{}, cancelError(ctx, step.id, runCleanups(ctx, state.cleanups))
		}

		emitProgress(cctx, stepIndex, step.id, StatusRunning, 0, "")
		sctx := newStepContext(ctx, stepProgressReporter(cctx, stepIndex, step.id))

		output, err := step.execute(sctx, state.output)
		if err != nil {
			return chainState[N]{}, stepError(step.id, err, runCleanups(ctx, state.cleanups))
		}

		emitProgress(cctx, stepIndex, step.id, StatusCompleted, 100, "")

		cleanups := state.cleanups
		if step.cleanup != nil {
			cleanups = append(cleanups, cleanupAction(step.cleanup))
		}
		return chainState[N]{output: output, cleanups: cleanups}, nil
	}

	return &Builder[I, N]{stepCount: b.stepCount + 1, runner: next}
}

// Build finalizes the builder into an executable Chain.
func (b *Builder[I, O]) Build() *Chain[I, O] {
	return &Chain[I, O]{stepCount: b.stepCount, runner: b.runner}
}

// emitProgress forwards a chain-level progress update when a callback is set.
func emitProgress(cctx chainContext, index int, stepID string, status StepStatus, percent uint8, message string) {
	if cctx.progress == nil {
		return
	}
	cctx.progress(StepProgress{
		StepIndex:       index,
		StepID:          stepID,
		Status:          status,
		ProgressPercent: percent,
		Message:         message,
	})
}

// stepProgressReporter adapts a chain-level callback into the (percent,
// message) reporter handed to a step's StepContext.
func stepProgressReporter(cctx chainContext, index int, stepID string) func(uint8, string) {
	if cctx.progress == nil {
		return nil
	}
	return func(percent uint8, message string) {
		cctx.progress(StepProgress{
			StepIndex:       index,
			StepID:          stepID,
			Status:          StatusRunning,
			ProgressPercent: percent,
			Message:         message,
		})
	}
}
