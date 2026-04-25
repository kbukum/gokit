package workload

import (
	"fmt"
	"sync"

	"github.com/kbukum/gokit/logger"
)

// ManagerFactory creates a Manager implementation from core config and
// provider-specific config. Each provider type-asserts providerCfg to its
// own config type.
type ManagerFactory func(cfg Config, providerCfg any, log *logger.Logger) (Manager, error)

// FactoryRegistry stores workload manager factories by provider name.
//
// Registries are explicit, isolated, and thread-safe. Use
// [NewFactoryRegistry] to create one, then call provider-specific
// Register functions (for example
// [github.com/kbukum/gokit/workload/docker.Register]) to populate it
// before passing it to [New].
type FactoryRegistry struct {
	mu        sync.RWMutex
	factories map[string]ManagerFactory
}

// NewFactoryRegistry creates an isolated workload factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{factories: make(map[string]ManagerFactory)}
}

// Register stores a workload backend factory for the given provider name.
// It returns an error if name or factory are invalid, or if a duplicate
// name is registered. This is a programmer error in production but is
// reported as a value to keep test code path-deterministic.
func (r *FactoryRegistry) Register(name string, f ManagerFactory) error {
	if name == "" {
		return fmt.Errorf("workload: provider name cannot be empty")
	}
	if f == nil {
		return fmt.Errorf("workload: factory %q cannot be nil", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("workload: factory %q already registered", name)
	}
	r.factories[name] = f
	return nil
}

// MustRegister is like [FactoryRegistry.Register] but panics on error.
// Intended for application startup where a failed registration is fatal.
func (r *FactoryRegistry) MustRegister(name string, f ManagerFactory) {
	if err := r.Register(name, f); err != nil {
		panic(err)
	}
}

// Get returns a workload factory by provider name.
func (r *FactoryRegistry) Get(name string) (ManagerFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[name]
	return f, ok
}

// Names returns the registered provider names in unspecified order.
func (r *FactoryRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for n := range r.factories {
		names = append(names, n)
	}
	return names
}

// New creates a workload Manager using the provided registry. The registry
// is mandatory: pass an explicit *FactoryRegistry with the desired
// providers registered (for example via docker.Register, kubernetes.Register).
func New(registry *FactoryRegistry, cfg Config, providerCfg any, log *logger.Logger) (Manager, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if registry == nil {
		return nil, fmt.Errorf("workload: factory registry is nil")
	}

	l := log.WithComponent("workload")

	f, ok := registry.Get(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("workload: unsupported provider %q (not registered)", cfg.Provider)
	}

	l.Info("initializing workload manager", map[string]interface{}{"provider": cfg.Provider})
	return f(cfg, providerCfg, l)
}
