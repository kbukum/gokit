package provider

import "context"

// Adapt wraps a RequestResponse provider with input/output type transformation.
// This bridges a backend service with types [BI, BO] to a domain interface with types [I, O].
//
// mapIn converts the domain input to the backend input.
// mapOut converts the backend output to the domain output.
//
// Adapt composes naturally with WithResilience, Stateful, and other middleware.
func Adapt[I, O, BI, BO any](
	inner RequestResponse[BI, BO],
	name string,
	mapIn func(ctx context.Context, input I) (BI, error),
	mapOut func(output BO) (O, error),
) RequestResponse[I, O] {
	return &adaptedRR[I, O, BI, BO]{
		inner:  inner,
		name:   name,
		mapIn:  mapIn,
		mapOut: mapOut,
	}
}

type adaptedRR[I, O, BI, BO any] struct {
	inner  RequestResponse[BI, BO]
	name   string
	mapIn  func(ctx context.Context, input I) (BI, error)
	mapOut func(output BO) (O, error)
}

func (a *adaptedRR[I, O, BI, BO]) Name() string { return a.name }

func (a *adaptedRR[I, O, BI, BO]) IsAvailable(ctx context.Context) bool {
	return a.inner.IsAvailable(ctx)
}

func (a *adaptedRR[I, O, BI, BO]) Execute(ctx context.Context, input I) (O, error) {
	var zero O

	backendInput, err := a.mapIn(ctx, input)
	if err != nil {
		return zero, err
	}

	backendOutput, err := a.inner.Execute(ctx, backendInput)
	if err != nil {
		return zero, err
	}

	return a.mapOut(backendOutput)
}
