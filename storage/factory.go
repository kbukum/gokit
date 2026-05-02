package storage

import (
	"fmt"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/provider"
)

// StorageFactory creates a Storage implementation from core config and
// provider-specific configuration. Each provider type-asserts providerCfg
// to its own config type.
type StorageFactory func(cfg Config, providerCfg any, log *logger.Logger) (Storage, error)

// FactoryRegistry stores storage factories by provider name.
type FactoryRegistry struct {
	inner *provider.NamedRegistry[StorageFactory]
}

// NewFactoryRegistry creates an isolated storage factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{inner: provider.NewNamedRegistry[StorageFactory]("storage")}
}

// Register stores a storage backend factory for the given provider name.
// It returns an error if name or factory are invalid, or if a duplicate
// name is registered.
//
// Note: prior versions panicked on duplicate; callers must now check the
// returned error.
func (r *FactoryRegistry) Register(name string, f StorageFactory) error {
	return r.inner.Register(name, f)
}

// Get returns a storage factory by provider name.
func (r *FactoryRegistry) Get(name string) (StorageFactory, bool) {
	return r.inner.Get(name)
}

// New creates a Storage implementation using the provided registry.
// The registry is mandatory; pass an explicit *FactoryRegistry with the desired
// provider registered (e.g. via local.Register, s3.Register, etc.).
func New(reg *FactoryRegistry, cfg Config, providerCfg any, log *logger.Logger) (Storage, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, fmt.Errorf("storage: factory registry is nil")
	}

	l := log.WithComponent("storage")

	f, ok := reg.Get(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("storage: unsupported provider %q (not registered)", cfg.Provider)
	}

	l.Debug("initializing storage", map[string]interface{}{"provider": cfg.Provider})
	return f(cfg, providerCfg, l)
}
