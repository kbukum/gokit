package cache

import (
	"fmt"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/provider/namedregistry"
)

// Factory creates a cache store from core and provider-specific config.
type Factory func(cfg Config, providerCfg any, log *logging.Logger) (Store, error)

// FactoryRegistry stores cache backend factories by provider name.
type FactoryRegistry struct {
	inner *namedregistry.Registry[Factory]
}

// NewFactoryRegistry creates an isolated cache factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{inner: namedregistry.New[Factory]("cache")}
}

// Register stores a cache backend factory for a provider name.
func (r *FactoryRegistry) Register(name string, f Factory) error {
	return r.inner.Register(name, f)
}

// Get returns a cache factory by provider name.
func (r *FactoryRegistry) Get(name string) (Factory, bool) {
	return r.inner.Get(name)
}

// New creates a Store using an explicitly populated registry.
func New(reg *FactoryRegistry, cfg Config, providerCfg any, log *logging.Logger) (Store, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, fmt.Errorf("cache: factory registry is nil")
	}
	f, ok := reg.Get(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("cache: unsupported provider %q (not registered)", cfg.Provider)
	}
	return f(cfg, providerCfg, log.WithComponent("cache"))
}
