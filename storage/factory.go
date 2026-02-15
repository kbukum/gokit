package storage

import (
	"fmt"

	"github.com/skillsenselab/gokit/logger"
)

// StorageFactory creates a Storage implementation from a Config.
type StorageFactory func(cfg Config) (Storage, error)

var factories = make(map[string]StorageFactory)

// RegisterFactory registers a storage backend factory for the given provider name.
// Implementation packages call this (typically in an init function) to make
// themselves available to the New constructor.
func RegisterFactory(name string, f StorageFactory) {
	factories[name] = f
}

// New creates a Storage implementation based on the given Config.
// The provider field determines which backend is used.
// Ensure the desired provider package has been imported (e.g.
// _ "github.com/skillsenselab/gokit/storage/local") so its factory is registered.
func New(cfg Config, log *logger.Logger) (Storage, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	l := log.WithComponent("storage")

	f, ok := factories[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("storage: unsupported provider %q (not registered)", cfg.Provider)
	}

	l.Info("initializing storage", map[string]interface{}{"provider": cfg.Provider})
	return f(cfg)
}
