package tool

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// AsProvider converts a typed Tool into a provider.RequestResponse.
// This bridges the tool system with the provider middleware stack (resilience, caching, tracing, etc.).
func (t *Tool[I, O]) AsProvider() provider.RequestResponse[I, O] {
	return &toolProvider[I, O]{tool: t}
}

type toolProvider[I, O any] struct {
	tool *Tool[I, O]
}

func (p *toolProvider[I, O]) Name() string { return p.tool.Def.Name }

func (p *toolProvider[I, O]) IsAvailable(_ context.Context) bool { return true }

func (p *toolProvider[I, O]) Execute(ctx context.Context, input I) (O, error) {
	return p.tool.handler.Execute(ctx, input)
}

// FromProvider creates a Tool from an existing provider.RequestResponse.
// The Definition must be provided since providers don't carry schema metadata.
func FromProvider[I, O any](def Definition, p provider.RequestResponse[I, O]) *Tool[I, O] {
	return &Tool[I, O]{
		Def:     def,
		handler: HandlerFunc[I, O](p.Execute),
	}
}
