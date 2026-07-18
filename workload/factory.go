package workload

import (
	"fmt"

	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/provider/namedregistry"
)

// ManagerFactory creates a Manager implementation from core config and provider-specific config.
// Each provider type-asserts providerCfg to its own config type.
type ManagerFactory func(cfg Config, providerCfg any, log *logging.Logger) (Manager, error)

// FactoryRegistry stores workload manager factories by provider name.
//
// Registries are explicit, isolated, and thread-safe. Use [NewFactoryRegistry] to create one,
// then call provider-specific Register functions (for example [github.com/kbukum/gokit/workload/docker.Register]) to populate it before passing it to [New].
type FactoryRegistry struct {
	inner *namedregistry.Registry[ManagerFactory]
}

// NewFactoryRegistry creates an isolated workload factory registry.
func NewFactoryRegistry() *FactoryRegistry {
	return &FactoryRegistry{inner: namedregistry.New[ManagerFactory]("workload")}
}

// Register stores a workload backend factory for the given provider name.
// It returns an error if name or factory are invalid, or if a duplicate name is registered.
func (r *FactoryRegistry) Register(name string, f ManagerFactory) error {
	return r.inner.Register(name, f)
}

// Get returns a workload factory by provider name.
func (r *FactoryRegistry) Get(name string) (ManagerFactory, bool) {
	return r.inner.Get(name)
}

// Names returns the registered provider names in deterministic (sorted) order.
func (r *FactoryRegistry) Names() []string {
	return r.inner.Names()
}

// New creates a workload Manager using the provided registry. The registry is mandatory:
// pass an explicit *FactoryRegistry with the desired providers registered (for example via docker.Register, kubernetes.Register).
func New(reg *FactoryRegistry, cfg Config, providerCfg any, log *logging.Logger) (Manager, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, fmt.Errorf("workload: factory registry is nil")
	}

	l := log.WithComponent("workload")

	f, ok := reg.Get(cfg.Provider)
	if !ok {
		return nil, fmt.Errorf("workload: unsupported provider %q (not registered)", cfg.Provider)
	}

	l.Info("initializing workload manager", map[string]any{"provider": cfg.Provider})
	return f(cfg, providerCfg, l)
}
