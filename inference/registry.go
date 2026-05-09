package inference

import (
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/provider/namedregistry"
)

// Factory builds an inference adapter from config.
type Factory func(config json.RawMessage) (Inference, error)

// Registry stores inference adapter factories by kind.
type Registry struct {
	inner *namedregistry.Registry[Factory]
}

// NewRegistry creates an empty inference registry. Backends register explicitly.
func NewRegistry() *Registry {
	return &Registry{inner: namedregistry.New[Factory]("inference")}
}

// Register adds an adapter factory under a stable kind.
func (r *Registry) Register(kind string, factory Factory) error {
	return r.inner.Register(kind, factory)
}

// Build constructs an adapter from a registered kind and config payload.
func (r *Registry) Build(kind string, config json.RawMessage) (Inference, error) {
	factory, err := r.inner.Lookup(kind)
	if err != nil {
		return nil, fmt.Errorf("inference: unknown adapter %q (forgot to register?): %w", kind, err)
	}
	provider, err := factory(config)
	if err != nil {
		return nil, fmt.Errorf("inference: build adapter %q: %w", kind, err)
	}
	return provider, nil
}

// Kinds returns the registered adapter kinds in stable order.
func (r *Registry) Kinds() []string { return r.inner.Names() }
