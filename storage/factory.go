package storage

import (
	"fmt"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/provider/namedregistry"
)

// StorageFactory creates a Storage implementation from core configuration.
// Provider-specific configuration is captured by the typed provider Register function,
// keeping runtime selection free of untyped config blobs.
type StorageFactory func(cfg Config, log *logging.Logger) (Storage, error)

// FactoryRegistry stores storage factories by provider name.
type FactoryRegistry struct {
	inner *namedregistry.Registry[StorageFactory]
}

// NewFactoryRegistry creates an isolated storage factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{inner: namedregistry.New[StorageFactory]("storage")}
}

// Register stores a configured storage backend factory for the given provider name.
func (r *FactoryRegistry) Register(name string, f StorageFactory) error {
	return r.inner.Register(name, f)
}

// Get returns a storage factory by provider name.
func (r *FactoryRegistry) Get(name string) (StorageFactory, bool) {
	return r.inner.Get(name)
}

// New creates a Storage implementation using the provided registry.
// Provider-specific settings are supplied by the typed provider Register call.
func New(reg *FactoryRegistry, cfg Config, log *logging.Logger) (Storage, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, fmt.Errorf("storage: factory registry is nil")
	}

	l := log
	if l == nil {
		l = logging.NewDefault("storage") //nolint:contextcheck // default logger construction has no request-scoped operation
	}
	l = l.WithComponent("storage")

	f, ok := reg.Get(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("storage: unsupported provider %q (not registered)", cfg.Provider)
	}

	l.Debug("initializing storage", map[string]any{"provider": cfg.Provider})
	return f(cfg, l)
}
