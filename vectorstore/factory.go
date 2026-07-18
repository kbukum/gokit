package vectorstore

import (
	"fmt"

	"github.com/kbukum/gokit/provider/namedregistry"
)

// Factory creates a Store from provider-agnostic config. Provider-specific configuration is captured by typed backend Register calls.
type Factory func(cfg Config) (Store, error)

// FactoryRegistry stores vectorstore factories by provider name.
type FactoryRegistry struct {
	inner *namedregistry.Registry[Factory]
}

// NewFactoryRegistry creates an isolated vectorstore factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{inner: namedregistry.New[Factory]("vectorstore")}
}

// Register stores a vectorstore backend factory for a provider name.
func (r *FactoryRegistry) Register(name string, f Factory) error {
	return r.inner.Register(name, f)
}

// Get returns a vectorstore factory by provider name.
func (r *FactoryRegistry) Get(name string) (Factory, bool) {
	return r.inner.Get(name)
}

// New creates a Store using the selected registered provider.
func New(reg *FactoryRegistry, cfg Config) (Store, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, fmt.Errorf("vectorstore: factory registry is nil")
	}
	f, ok := reg.Get(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("vectorstore: unsupported provider %q (not registered)", cfg.Provider)
	}
	return f(cfg)
}

// RegisterMemory registers the core in-memory backend.
func RegisterMemory(reg *FactoryRegistry) error {
	return reg.Register(ProviderMemory, func(cfg Config) (Store, error) {
		return NewInMemoryStoreWithConfig(cfg)
	})
}
