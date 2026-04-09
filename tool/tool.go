package tool

import "context"

// Handler processes tool input and produces output.
type Handler[I, O any] interface {
	Execute(ctx context.Context, input I) (O, error)
}

// HandlerFunc is the function adapter for [Handler].
// It allows using a plain function where a Handler is expected.
type HandlerFunc[I, O any] func(ctx context.Context, input I) (O, error)

// Execute implements [Handler].
func (f HandlerFunc[I, O]) Execute(ctx context.Context, input I) (O, error) {
	return f(ctx, input)
}

// Tool is a typed, executable capability with auto-generated schemas.
type Tool[I, O any] struct {
	Def     Definition
	handler Handler[I, O]
}

// NewTool creates a Tool with an explicit Definition and Handler.
func NewTool[I, O any](def Definition, handler Handler[I, O]) *Tool[I, O] {
	return &Tool[I, O]{
		Def:     def,
		handler: handler,
	}
}

// Execute runs the tool with typed input.
func (t *Tool[I, O]) Execute(ctx context.Context, input I) (O, error) {
	return t.handler.Execute(ctx, input)
}

// WithAnnotations returns a copy of the tool with the given annotations.
func (t *Tool[I, O]) WithAnnotations(a Annotations) *Tool[I, O] {
	t.Def.Annotations = &a
	return t
}

// Definition returns the tool's definition.
func (t *Tool[I, O]) Definition() Definition {
	return t.Def
}
