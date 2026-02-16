package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/logger"
)

// Manager provides the main API for working with providers,
// combining a Registry for storage and a Selector for choosing providers.
type Manager[T Provider] struct {
	mu          sync.RWMutex
	registry    *Registry[T]
	selector    Selector[T]
	providers   map[string]T
	defaultName string
	log         *logger.Logger
}

// NewManager creates a Manager backed by the given registry and selector.
func NewManager[T Provider](registry *Registry[T], selector Selector[T]) *Manager[T] {
	return &Manager[T]{
		registry:  registry,
		selector:  selector,
		providers: make(map[string]T),
		log:       logger.Get("provider"),
	}
}

// Register adds a factory to the underlying registry.
func (m *Manager[T]) Register(name string, factory Factory[T]) {
	m.registry.RegisterFactory(name, factory)
	m.log.Info("factory registered", map[string]interface{}{"provider": name})
}

// Initialize creates a provider from its factory and stores it for use.
func (m *Manager[T]) Initialize(name string, cfg map[string]any) error {
	instance, err := m.registry.Create(name, cfg)
	if err != nil {
		return fmt.Errorf("initialize provider %q: %w", name, err)
	}
	m.mu.Lock()
	m.providers[name] = instance
	m.mu.Unlock()
	m.registry.Set(name, instance)
	m.log.Info("provider initialized", map[string]interface{}{"provider": name})
	return nil
}

// Get returns a provider chosen by the selector, or the default if set.
func (m *Manager[T]) Get(ctx context.Context) (T, error) {
	m.mu.RLock()
	defaultName := m.defaultName
	providers := m.snapshotLocked()
	m.mu.RUnlock()

	if defaultName != "" {
		if p, ok := providers[defaultName]; ok {
			return p, nil
		}
		var zero T
		return zero, fmt.Errorf("default provider %q not found", defaultName)
	}
	return m.selector.Select(ctx, providers)
}

// GetByName returns a specific provider by name.
func (m *Manager[T]) GetByName(name string) (T, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.providers[name]; ok {
		return p, nil
	}
	var zero T
	return zero, fmt.Errorf("provider %q not found", name)
}

// SetDefault sets the default provider by name.
func (m *Manager[T]) SetDefault(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.providers[name]; !ok {
		return fmt.Errorf("provider %q not initialized", name)
	}
	m.defaultName = name
	m.log.Info("default provider set", map[string]interface{}{"provider": name})
	return nil
}

// Available returns the names of all initialized providers.
func (m *Manager[T]) Available() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.providers))
	for name := range m.providers {
		names = append(names, name)
	}
	return names
}

// snapshotLocked returns a shallow copy of the providers map.
// Must be called while holding at least a read lock.
func (m *Manager[T]) snapshotLocked() map[string]T {
	cp := make(map[string]T, len(m.providers))
	for k, v := range m.providers {
		cp[k] = v
	}
	return cp
}
