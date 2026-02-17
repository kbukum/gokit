package workload

import (
	"fmt"

	"github.com/kbukum/gokit/logger"
)

// ManagerFactory creates a Manager implementation from core config and provider-specific config.
type ManagerFactory func(cfg Config, providerCfg any, log *logger.Logger) (Manager, error)

var factories = make(map[string]ManagerFactory)

// RegisterFactory registers a workload provider factory.
func RegisterFactory(name string, f ManagerFactory) {
	factories[name] = f
}

// New creates a Manager for the configured provider.
func New(cfg Config, providerCfg any, log *logger.Logger) (Manager, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	l := log.WithComponent("workload")

	f, ok := factories[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("workload: unsupported provider %q (not registered)", cfg.Provider)
	}

	l.Info("initializing workload manager", map[string]interface{}{"provider": cfg.Provider})
	return f(cfg, providerCfg, l)
}
