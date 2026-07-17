package stage

import "context"

// Transform maps an input value to an optional output value. Returning keep as
// false drops the item from the stream (a filter), letting one stage both map
// and filter.
type Transform[I, O any] interface {
	// Name returns a stable identifier for diagnostics.
	Name() string
	// Apply transforms in, returning the output and whether to keep it, or a
	// typed error that aborts the stream.
	Apply(ctx context.Context, in I) (out O, keep bool, err error)
}

// TransformFunc adapts a function into a [Transform].
type TransformFunc[I, O any] struct {
	// FuncName is the transform's stable identifier.
	FuncName string
	// Fn performs the mapping.
	Fn func(ctx context.Context, in I) (O, bool, error)
}

// Name returns the transform's identifier.
func (t TransformFunc[I, O]) Name() string { return t.FuncName }

// Apply invokes the wrapped function.
func (t TransformFunc[I, O]) Apply(ctx context.Context, in I) (out O, keep bool, err error) {
	return t.Fn(ctx, in)
}
