package bench

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// Evaluator is a provider.RequestResponse that produces predictions from raw input.
type Evaluator[L comparable] interface {
	provider.RequestResponse[[]byte, Prediction[L]]
}

// evaluatorFunc wraps a plain function as an Evaluator.
type evaluatorFunc[L comparable] struct {
	name string
	fn   func(ctx context.Context, input []byte) (Prediction[L], error)
}

// EvaluatorFunc wraps a plain function as an Evaluator.
func EvaluatorFunc[L comparable](name string, fn func(ctx context.Context, input []byte) (Prediction[L], error)) Evaluator[L] {
	return &evaluatorFunc[L]{name: name, fn: fn}
}

func (e *evaluatorFunc[L]) Name() string                       { return e.name }
func (e *evaluatorFunc[L]) IsAvailable(_ context.Context) bool { return true }
func (e *evaluatorFunc[L]) Execute(ctx context.Context, input []byte) (Prediction[L], error) {
	return e.fn(ctx, input)
}

// fromProvider adapts any RequestResponse provider into an Evaluator.
type fromProvider[I, O any, L comparable] struct {
	p            provider.RequestResponse[I, O]
	toInput      func([]byte) I
	toPrediction func(O) Prediction[L]
}

// FromProvider adapts any RequestResponse provider into an Evaluator
// using mapper functions for input/output transformation.
func FromProvider[I, O any, L comparable](
	p provider.RequestResponse[I, O],
	toInput func([]byte) I,
	toPrediction func(O) Prediction[L],
) Evaluator[L] {
	return &fromProvider[I, O, L]{p: p, toInput: toInput, toPrediction: toPrediction}
}

func (a *fromProvider[I, O, L]) Name() string                       { return a.p.Name() }
func (a *fromProvider[I, O, L]) IsAvailable(ctx context.Context) bool { return a.p.IsAvailable(ctx) }
func (a *fromProvider[I, O, L]) Execute(ctx context.Context, input []byte) (Prediction[L], error) {
	out, err := a.p.Execute(ctx, a.toInput(input))
	if err != nil {
		var zero Prediction[L]
		return zero, err
	}
	return a.toPrediction(out), nil
}
