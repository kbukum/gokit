package component

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/logger"
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

// StartAll starts all not-yet-started components in registration order.
//
// It is safe to call multiple times — already-started components are
// skipped. This supports two-phase startup where infrastructure
// components are started first and application-layer components
// (registered during configure) are started in a second pass.
//
// If a component fails to start, all components that were successfully
// started during this call are rolled back (stopped in reverse order).
// Components started by a previous call are NOT rolled back.
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Collect indices of components that need starting.
	var pending []int
	for i, entry := range r.entries {
		if !entry.started {
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 {
		return nil
	}

	logger.Debug("Starting components", map[string]interface{}{
		"pending": len(pending),
		"total":   len(r.entries),
	})

	// Track which entries we start in this call for rollback.
	var startedThisCall []int

	for _, idx := range pending {
		entry := r.entries[idx]
		name := entry.component.Name()

		logger.Debug("Starting component", map[string]interface{}{"component": name})
		if err := entry.component.Start(ctx); err != nil {
			logger.Error("Component start failed", map[string]interface{}{
				"component": name,
				"error":     err.Error(),
			})
			// Rollback: stop components started during this call (reverse order).
			r.rollbackLocked(ctx, startedThisCall)
			return fmt.Errorf("failed to start %s: %w", name, err)
		}

		entry.started = true
		startedThisCall = append(startedThisCall, idx)
		logger.Debug("Component started", map[string]interface{}{"component": name})
	}

	logger.Info("All components started successfully")
	return nil
}

// rollbackLocked stops the entries at the given indices in reverse order.
// Caller must hold r.mu.
func (r *Registry) rollbackLocked(ctx context.Context, indices []int) {
	for i := len(indices) - 1; i >= 0; i-- {
		entry := r.entries[indices[i]]
		name := entry.component.Name()
		stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := entry.component.Stop(stopCtx); err != nil {
			logger.Error("Rollback stop failed", map[string]interface{}{
				"component": name,
				"error":     err.Error(),
			})
		}
		entry.started = false
		cancel()
	}
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
func (r *Registry) HealthAll(ctx context.Context) []Health {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]Health, 0, len(r.entries))
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
