package provider

import "context"

// TapSink wraps a Sink with a side-effect function that runs before sending.
// The tap function observes the input without modifying it. If the tap
// returns an error, the input is still forwarded to the inner sink.
//
// Use for metrics, logging, or feeding derived processors inline.
func TapSink[I any](inner Sink[I], tap func(context.Context, I)) Sink[I] {
	return &tapSink[I]{inner: inner, tap: tap}
}

type tapSink[I any] struct {
	inner Sink[I]
	tap   func(context.Context, I)
}

func (t *tapSink[I]) Name() string                         { return t.inner.Name() }
func (t *tapSink[I]) IsAvailable(ctx context.Context) bool { return t.inner.IsAvailable(ctx) }

func (t *tapSink[I]) Send(ctx context.Context, input I) error {
	t.tap(ctx, input)
	return t.inner.Send(ctx, input)
}
