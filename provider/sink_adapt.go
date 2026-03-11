package provider

import "context"

// AdaptSink wraps a Sink with input type transformation.
// This bridges a domain type I to a backend type BI before sending.
//
// mapIn converts the domain input to the backend input.
//
// AdaptSink composes naturally with FanOutSink, WithSinkResilience,
// and other sink combinators.
func AdaptSink[I, BI any](
	inner Sink[BI],
	name string,
	mapIn func(ctx context.Context, input I) (BI, error),
) Sink[I] {
	return &adaptedSink[I, BI]{
		inner: inner,
		name:  name,
		mapIn: mapIn,
	}
}

type adaptedSink[I, BI any] struct {
	inner Sink[BI]
	name  string
	mapIn func(ctx context.Context, input I) (BI, error)
}

func (a *adaptedSink[I, BI]) Name() string { return a.name }

func (a *adaptedSink[I, BI]) IsAvailable(ctx context.Context) bool {
	return a.inner.IsAvailable(ctx)
}

func (a *adaptedSink[I, BI]) Send(ctx context.Context, input I) error {
	backendInput, err := a.mapIn(ctx, input)
	if err != nil {
		return err
	}
	return a.inner.Send(ctx, backendInput)
}
