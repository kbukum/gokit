package provider

import "context"

// SinkFunc wraps a plain function as a Sink provider.
// This is the Sink equivalent of http.HandlerFunc — it adapts any
// func(context.Context, I) error into the full Sink[I] interface.
type SinkFunc[I any] struct {
	name string
	fn   func(context.Context, I) error
}

// NewSinkFunc creates a Sink from a plain function.
func NewSinkFunc[I any](name string, fn func(context.Context, I) error) Sink[I] {
	return &SinkFunc[I]{name: name, fn: fn}
}

func (s *SinkFunc[I]) Name() string                            { return s.name }
func (s *SinkFunc[I]) IsAvailable(_ context.Context) bool      { return true }
func (s *SinkFunc[I]) Send(ctx context.Context, input I) error { return s.fn(ctx, input) }
