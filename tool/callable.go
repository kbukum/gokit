package tool

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/schema"
)

// Callable is the type-erased interface for tools that can be stored
// in heterogeneous collections (registries). It accepts raw JSON input
// and returns a structured Result.
type Callable interface {
	// Definition returns the tool's metadata.
	Definition() Definition
	// Validate checks the input against the tool's input schema.
	Validate(input json.RawMessage) schema.ValidationResult
	// Call executes the tool with JSON input and returns a structured Result.
	Call(ctx *Context, input json.RawMessage) (*Result, error)
}

// AsCallable converts a typed Tool[I,O] into a Callable by adding
// JSON marshalling/unmarshalling around the typed handler.
func (t *Tool[I, O]) AsCallable() Callable {
	return &wrappedCallable[I, O]{tool: t}
}

type wrappedCallable[I, O any] struct {
	tool *Tool[I, O]
}

func (w *wrappedCallable[I, O]) Definition() Definition {
	return w.tool.Def
}

func (w *wrappedCallable[I, O]) Validate(input json.RawMessage) schema.ValidationResult {
	return schema.Validate(w.tool.Def.InputSchema, input)
}

func (w *wrappedCallable[I, O]) Call(ctx *Context, input json.RawMessage) (*Result, error) {
	var in I
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return nil, fmt.Errorf("tool %q: unmarshal input: %w", w.tool.Def.Name, err)
		}
	}

	out, err := w.tool.handler.Execute(ctx, in)
	if err != nil {
		return nil, err
	}

	return JSONResult(out)
}
