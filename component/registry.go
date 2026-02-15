package component

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/skillsenselab/gokit/logger"
)

// componentEntry holds a component and its started state.
type componentEntry struct {
	component Component
	started   bool
}

// Registry manages component lifecycle with deterministic ordering.
// Components are started in registration order and stopped in reverse order.
type Registry struct {
	entries []*componentEntry
	lookup  map[string]*componentEntry
	mu      sync.RWMutex
}

// NewRegistry creates a new component registry.
func NewRegistry() *Registry {
	return &Registry{
		entries: make([]*componentEntry, 0),
		lookup:  make(map[string]*componentEntry),
	}
}

// Register adds a component to the registry. Components are started in
// the order they are registered, so register dependencies first.
func (r *Registry) Register(c Component) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := c.Name()
	if _, exists := r.lookup[name]; exists {
		return fmt.Errorf("component %s already registered", name)
	}

	entry := &componentEntry{component: c}
	r.entries = append(r.entries, entry)
	r.lookup[name] = entry

	logger.Debug("Component registered", map[string]interface{}{
		"component": name,
	})
	return nil
}

// StartAll starts all components in registration order.
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	logger.Info("Starting all components", map[string]interface{}{
		"count": len(r.entries),
	})

	for _, entry := range r.entries {
		name := entry.component.Name()

		logger.Debug("Starting component", map[string]interface{}{"component": name})
		if err := entry.component.Start(ctx); err != nil {
			logger.Error("Component start failed", map[string]interface{}{
				"component": name,
				"error":     err.Error(),
			})
			return fmt.Errorf("failed to start %s: %w", name, err)
		}

		entry.started = true
		logger.Debug("Component started", map[string]interface{}{"component": name})
	}

	logger.Info("All components started successfully")
	return nil
}

// StopAll gracefully stops all components in reverse registration order.
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	logger.Info("Stopping all components")

	var errs []error
	for i := len(r.entries) - 1; i >= 0; i-- {
		entry := r.entries[i]
		if !entry.started {
			continue
		}

		name := entry.component.Name()
		logger.Debug("Stopping component", map[string]interface{}{"component": name})

		stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := entry.component.Stop(stopCtx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop %s: %w", name, err))
			logger.Error("Component stop failed", map[string]interface{}{
				"component": name,
				"error":     err.Error(),
			})
		} else {
			logger.Info("Component stopped", map[string]interface{}{"component": name})
		}
		entry.started = false
		cancel()
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	logger.Info("All components stopped successfully")
	return nil
}

// HealthAll returns health status for all registered components.
func (r *Registry) HealthAll(ctx context.Context) []ComponentHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]ComponentHealth, 0, len(r.entries))
	for _, entry := range r.entries {
		results = append(results, entry.component.Health(ctx))
	}
	return results
}

// Get returns a registered component by name, or nil if not found.
func (r *Registry) Get(name string) Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, exists := r.lookup[name]; exists {
		return entry.component
	}
	return nil
}

// All returns all registered components in registration order.
func (r *Registry) All() []Component {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Component, 0, len(r.entries))
	for _, entry := range r.entries {
		result = append(result, entry.component)
	}
	return result
}
