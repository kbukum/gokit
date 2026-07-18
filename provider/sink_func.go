package provider

import (
	"context"
	"errors"
)

// ErrNilSinkFunc is returned by SinkFunc.Send when the wrapper was constructed with a nil function. Sends fail closed with this typed error rather than panicking on the runtime path.
var ErrNilSinkFunc = errors.New("provider: SinkFunc has nil function")

// SinkFunc wraps a plain function as a Sink provider.
// This is the Sink equivalent of http.HandlerFunc — it adapts any
// func(context.Context, I) error into the full Sink[I] interface.
type SinkFunc[I any] struct {
	name string
	fn   func(context.Context, I) error
}

// NewSinkFunc creates a Sink from a plain function. A nil fn is tolerated at construction; Send then fails closed with ErrNilSinkFunc instead of panicking.
func NewSinkFunc[I any](name string, fn func(context.Context, I) error) Sink[I] {
	return &SinkFunc[I]{name: name, fn: fn}
}

func (s *SinkFunc[I]) Name() string { return s.name }

// IsAvailable reports whether the sink can accept sends. A SinkFunc built with a nil function is never available, since Send always fails with [ErrNilSinkFunc].
func (s *SinkFunc[I]) IsAvailable(_ context.Context) bool { return s.fn != nil }

func (s *SinkFunc[I]) Send(ctx context.Context, input I) error {
	if s.fn == nil {
		return ErrNilSinkFunc
	}
	return s.fn(ctx, input)
}
