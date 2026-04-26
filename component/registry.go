package component

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/logger"
)

// DefaultStopTimeout is applied to a component's Stop call only when the
// caller-supplied context has no deadline. A bounded fallback prevents a
// stuck Stop from blocking shutdown forever, while still letting callers
// pass a tighter deadline by attaching one to ctx.
const DefaultStopTimeout = 10 * time.Second

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
	// lifecycleMu serializes StartAll / StopAll against each other and
	// against themselves so callers cannot interleave a boot and a shutdown.
	// It is held for the duration of those operations, but does NOT block
	// concurrent reads (Get/All/HealthAll) or new Register calls.
	lifecycleMu sync.Mutex
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
//
// The Component.Start call runs without holding any registry lock so
// readers (Get / All / HealthAll) and concurrent Register calls are not
// blocked for the duration of the boot sequence.
func (r *Registry) StartAll(ctx context.Context) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	// Snapshot entries that need starting under the read lock.
	r.mu.RLock()
	pending := make([]*componentEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		if !entry.started {
			pending = append(pending, entry)
		}
	}
	total := len(r.entries)
	r.mu.RUnlock()

	if len(pending) == 0 {
		return nil
	}

	logger.Debug("Starting components", map[string]interface{}{
		"pending": len(pending),
		"total":   total,
	})

	// Track entries started in this call for rollback.
	startedThisCall := make([]*componentEntry, 0, len(pending))

	for _, entry := range pending {
		name := entry.component.Name()

		logger.Debug("Starting component", map[string]interface{}{"component": name})
		if err := entry.component.Start(ctx); err != nil {
			logger.Error("Component start failed", map[string]interface{}{
				"component": name,
				"error":     err.Error(),
			})
			r.rollback(ctx, startedThisCall)
			return fmt.Errorf("failed to start %s: %w", name, err)
		}

		r.mu.Lock()
		entry.started = true
		r.mu.Unlock()
		startedThisCall = append(startedThisCall, entry)

		logger.Debug("Component started", map[string]interface{}{"component": name})
	}

	logger.Info("All components started successfully")
	return nil
}

// rollback stops the given entries in reverse order. lifecycleMu is already
// held by the caller (StartAll); no other Start/Stop can interleave.
func (r *Registry) rollback(ctx context.Context, entries []*componentEntry) {
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		name := entry.component.Name()
		stopCtx, cancel := stopContext(ctx)
		if err := entry.component.Stop(stopCtx); err != nil {
			logger.Error("Rollback stop failed", map[string]interface{}{
				"component": name,
				"error":     err.Error(),
			})
		}
		cancel()
		r.mu.Lock()
		entry.started = false
		r.mu.Unlock()
	}
}

// StopAll gracefully stops all components in reverse registration order.
//
// Each Component.Stop runs with the caller's ctx; if ctx has no deadline,
// DefaultStopTimeout is applied per-component as a safety net (closes
// F-075: hardcoded shutdown deadlines no longer clobber a caller-supplied
// deadline).
func (r *Registry) StopAll(ctx context.Context) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	// Snapshot started entries (reverse order) under the read lock.
	r.mu.RLock()
	toStop := make([]*componentEntry, 0, len(r.entries))
	for i := len(r.entries) - 1; i >= 0; i-- {
		if r.entries[i].started {
			toStop = append(toStop, r.entries[i])
		}
	}
	r.mu.RUnlock()

	logger.Info("Stopping all components")

	var errs []error
	for _, entry := range toStop {
		name := entry.component.Name()
		logger.Debug("Stopping component", map[string]interface{}{"component": name})

		stopCtx, cancel := stopContext(ctx)
		if err := entry.component.Stop(stopCtx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop %s: %w", name, err))
			logger.Error("Component stop failed", map[string]interface{}{
				"component": name,
				"error":     err.Error(),
			})
		} else {
			logger.Info("Component stopped", map[string]interface{}{"component": name})
		}
		cancel()

		r.mu.Lock()
		entry.started = false
		r.mu.Unlock()
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}

	logger.Info("All components stopped successfully")
	return nil
}

// stopContext returns a context for an individual Component.Stop call. If
// the parent already has a deadline, it is used as-is. Otherwise a
// DefaultStopTimeout is applied as a bounded safety net.
func stopContext(parent context.Context) (context.Context, context.CancelFunc) {
	if _, ok := parent.Deadline(); ok {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, DefaultStopTimeout)
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
