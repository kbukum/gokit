package storage

import (
	"fmt"
	"sync"

	"github.com/kbukum/gokit/logger"
)

// StorageFactory creates a Storage implementation from core config and
// provider-specific configuration. Each provider type-asserts providerCfg
// to its own config type.
type StorageFactory func(cfg Config, providerCfg any, log *logger.Logger) (Storage, error)

// FactoryRegistry stores storage factories by provider name.
type FactoryRegistry struct {
	mu        sync.RWMutex
	factories map[string]StorageFactory
}

// NewFactoryRegistry creates an isolated storage factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{factories: make(map[string]StorageFactory)}
}

// Register stores a storage backend factory for the given provider name.
// It panics if name or factory are invalid, or if a duplicate name is registered.
func (r *FactoryRegistry) Register(name string, f StorageFactory) {
	if name == "" {
		panic("storage: provider name cannot be empty")
	}
	if f == nil {
		panic(fmt.Sprintf("storage: factory %q cannot be nil", name))
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		panic(fmt.Sprintf("storage: factory %q already registered", name))
	}
	r.factories[name] = f
}

// Get returns a storage factory by provider name.
func (r *FactoryRegistry) Get(name string) (StorageFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[name]
	return f, ok
}

// DefaultFactoryRegistry is a package-level compatibility shim.
// New code should prefer injecting an explicit registry into composition roots.
var DefaultFactoryRegistry = NewFactoryRegistry()

// RegisterFactory registers a storage backend factory in DefaultFactoryRegistry.
// Implementation packages often call this from init() for backward compatibility.
func RegisterFactory(name string, f StorageFactory) {
	DefaultFactoryRegistry.Register(name, f)
}

// New creates a Storage implementation based on the given Config.
// The provider field determines which backend is used.
// providerCfg carries provider-specific settings (e.g. *s3.Config, *supabase.Config).
// Ensure the desired provider package has been imported (e.g.
// _ "github.com/kbukum/gokit/storage/local") so its factory is registered.
func New(cfg Config, providerCfg any, log *logger.Logger) (Storage, error) {
	return NewWithRegistry(DefaultFactoryRegistry, cfg, providerCfg, log)
}

// NewWithRegistry creates a Storage implementation using the provided registry.
func NewWithRegistry(registry *FactoryRegistry, cfg Config, providerCfg any, log *logger.Logger) (Storage, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if registry == nil {
		return nil, fmt.Errorf("storage: factory registry is nil")
	}

	l := log.WithComponent("storage")

	f, ok := registry.Get(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("storage: unsupported provider %q (not registered)", cfg.Provider)
	}

	l.Debug("initializing storage", map[string]interface{}{"provider": cfg.Provider})
	return f(cfg, providerCfg, l)
}
