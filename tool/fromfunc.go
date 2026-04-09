package tool

import (
	"context"

	"github.com/kbukum/gokit/schema"
)

// FromFunc creates a Tool from a typed function, auto-generating input
// and output JSON Schemas from the function's type parameters.
//
// This is the primary way to create tools — minimal boilerplate:
//
//	searchTool := tool.FromFunc("search", "Search content", doSearch)
//
// Where doSearch is: func(ctx context.Context, in SearchInput) (SearchOutput, error)
func FromFunc[I, O any](name, description string, fn func(ctx context.Context, input I) (O, error)) *Tool[I, O] {
	return &Tool[I, O]{
		Def: Definition{
			Name:         name,
			Description:  description,
			InputSchema:  schema.Generate[I](),
			OutputSchema: schema.Generate[O](),
		},
		handler: HandlerFunc[I, O](fn),
	}
}

// FromFuncInputOnly creates a Tool that auto-generates only the input schema.
// Use when the output type is dynamic or not useful for schema generation
// (e.g., map[string]any).
func FromFuncInputOnly[I, O any](name, description string, fn func(ctx context.Context, input I) (O, error)) *Tool[I, O] {
	return &Tool[I, O]{
		Def: Definition{
			Name:        name,
			Description: description,
			InputSchema: schema.Generate[I](),
		},
		handler: HandlerFunc[I, O](fn),
	}
}
