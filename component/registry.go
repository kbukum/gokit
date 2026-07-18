package component

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/logging"
)

// DefaultStopTimeout is applied to a component's Stop call only when the caller-supplied context has no deadline. A bounded fallback prevents a stuck Stop from blocking shutdown forever, while still letting callers pass a tighter deadline by attaching one to ctx.
const DefaultStopTimeout = 10 * time.Second

// componentEntry holds a component and its lifecycle state.
type componentEntry struct {
	component Component
	state     State
}

// Registry manages component lifecycle with deterministic ordering. Components are started in registration order and stopped in reverse order. Each component tracks a formal lifecycle state (Created → Starting → Running → Stopping → Stopped | Failed).
type Registry struct {
	entries []*componentEntry
	lookup  map[string]*componentEntry
	mu      sync.RWMutex
	// lifecycleMu serializes StartAll / StopAll against each other and against themselves so callers cannot interleave a boot and a shutdown. It is held for the duration of those operations, but does NOT block concurrent reads (Get/All/HealthAll) or new Register calls.
	lifecycleMu sync.Mutex
}

// NewRegistry creates a new component registry.
func NewRegistry() *Registry {
	return &Registry{
		entries: make([]*componentEntry, 0),
		lookup:  make(map[string]*componentEntry),
	}
}

// Register adds a component to the registry. Components are started in the order they are registered, so register dependencies first.
func (r *Registry) Register(c Component) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := c.Name()
	if _, exists := r.lookup[name]; exists {
		return fmt.Errorf("component %s already registered", name)
	}

	entry := &componentEntry{component: c, state: StateCreated}
	r.entries = append(r.entries, entry)
	r.lookup[name] = entry

	logging.Debug("Component registered", map[string]any{
		"component": name,
	})
	return nil
}

// State returns the lifecycle state of a named component. Returns StateCreated and false if the component is not registered.
func (r *Registry) State(name string) (State, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if entry, exists := r.lookup[name]; exists {
		return entry.state, true
	}
	return StateCreated, false
}

// StartAll starts all not-yet-started components in registration order.
//
// It is safe to call multiple times — already-running components are skipped. This supports two-phase startup where infrastructure components are started first and application-layer components (registered during configure) are started in a second pass.
//
// If a component fails to start, all components that were successfully started during this call are rolled back (stopped in reverse order). Components started by a previous call are NOT rolled back.
//
// The Component.Start call runs without holding any registry lock so readers (Get / All / HealthAll) and concurrent Register calls are not blocked for the duration of the boot sequence.
func (r *Registry) StartAll(ctx context.Context) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	// Snapshot entries that need starting under the read lock.
	r.mu.RLock()
	pending := make([]*componentEntry, 0, len(r.entries))
	for _, entry := range r.entries {
		if entry.state == StateCreated || entry.state == StateStopped || entry.state == StateFailed {
			pending = append(pending, entry)
		}
	}
	total := len(r.entries)
	r.mu.RUnlock()

	if len(pending) == 0 {
		return nil
	}

	logging.DebugCtx(ctx, "Starting components", map[string]any{
		"pending": len(pending),
		"total":   total,
	})

	// Track entries started in this call for rollback.
	startedThisCall := make([]*componentEntry, 0, len(pending))

	for _, entry := range pending {
		name := entry.component.Name()

		r.mu.Lock()
		entry.state = StateStarting
		r.mu.Unlock()

		logging.DebugCtx(ctx, "Starting component", map[string]any{"component": name})
		if err := entry.component.Start(ctx); err != nil {
			r.mu.Lock()
			entry.state = StateFailed
			r.mu.Unlock()

			logging.ErrorCtx(ctx, "Component start failed", map[string]any{
				"component": name,
				"error":     err.Error(),
			})
			r.rollback(ctx, startedThisCall)
			return fmt.Errorf("failed to start %s: %w", name, err)
		}

		r.mu.Lock()
		entry.state = StateRunning
		r.mu.Unlock()
		startedThisCall = append(startedThisCall, entry)

		logging.DebugCtx(ctx, "Component started", map[string]any{"component": name})
	}

	logging.InfoCtx(ctx, "All components started successfully")
	return nil
}

// rollback stops the given entries in reverse order. lifecycleMu is already held by the caller (StartAll); no other Start/Stop can interleave.
func (r *Registry) rollback(ctx context.Context, entries []*componentEntry) {
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		name := entry.component.Name()

		r.mu.Lock()
		entry.state = StateStopping
		r.mu.Unlock()

		stopCtx, cancel := stopContext(ctx)
		if err := entry.component.Stop(stopCtx); err != nil {
			logging.ErrorCtx(ctx, "Rollback stop failed", map[string]any{
				"component": name,
				"error":     err.Error(),
			})
		}
		cancel()

		r.mu.Lock()
		entry.state = StateStopped
		r.mu.Unlock()
	}
}

// StopAll gracefully stops all running components in reverse registration order.
//
// Each Component.Stop runs with the caller's ctx; if ctx has no deadline, DefaultStopTimeout is applied per-component as a safety net. Errors are aggregated via errors.Join so callers can inspect individual failures with errors.Is/errors.As.
func (r *Registry) StopAll(ctx context.Context) error {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	// Snapshot started entries (reverse order) under the read lock.
	r.mu.RLock()
	toStop := make([]*componentEntry, 0, len(r.entries))
	for i := len(r.entries) - 1; i >= 0; i-- {
		if r.entries[i].state == StateRunning {
			toStop = append(toStop, r.entries[i])
		}
	}
	r.mu.RUnlock()

	logging.InfoCtx(ctx, "Stopping all components")

	var errs []error
	for _, entry := range toStop {
		name := entry.component.Name()

		r.mu.Lock()
		entry.state = StateStopping
		r.mu.Unlock()

		logging.DebugCtx(ctx, "Stopping component", map[string]any{"component": name})

		stopCtx, cancel := stopContext(ctx)
		if err := entry.component.Stop(stopCtx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop %s: %w", name, err))
			logging.ErrorCtx(ctx, "Component stop failed", map[string]any{
				"component": name,
				"error":     err.Error(),
			})
		} else {
			logging.InfoCtx(ctx, "Component stopped", map[string]any{"component": name})
		}
		cancel()

		r.mu.Lock()
		entry.state = StateStopped
		r.mu.Unlock()
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	logging.InfoCtx(ctx, "All components stopped successfully")
	return nil
}

// StopAllDetailed gracefully stops all running components and returns per-component results. This provides structured error information for callers that need to know which specific components failed.
func (r *Registry) StopAllDetailed(ctx context.Context) []StopResult {
	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.mu.RLock()
	toStop := make([]*componentEntry, 0, len(r.entries))
	for i := len(r.entries) - 1; i >= 0; i-- {
		if r.entries[i].state == StateRunning {
			toStop = append(toStop, r.entries[i])
		}
	}
	r.mu.RUnlock()

	results := make([]StopResult, 0, len(toStop))
	for _, entry := range toStop {
		name := entry.component.Name()

		r.mu.Lock()
		entry.state = StateStopping
		r.mu.Unlock()

		stopCtx, cancel := stopContext(ctx)
		err := entry.component.Stop(stopCtx)
		cancel()

		r.mu.Lock()
		entry.state = StateStopped
		r.mu.Unlock()

		results = append(results, StopResult{Name: name, Err: err})
	}
	return results
}

// stopContext returns a context for an individual Component.Stop call. If the parent already has a deadline, it is used as-is. Otherwise a DefaultStopTimeout is applied as a bounded safety net.
func stopContext(parent context.Context) (context.Context, context.CancelFunc) {
	if _, ok := parent.Deadline(); ok {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, DefaultStopTimeout)
}

// HealthAll returns health status for all registered components. The snapshot is taken under the read lock, but Health() calls are made without holding the lock to avoid blocking registration or lifecycle ops.
func (r *Registry) HealthAll(ctx context.Context) []Health {
	r.mu.RLock()
	snapshot := make([]Component, len(r.entries))
	for i, entry := range r.entries {
		snapshot[i] = entry.component
	}
	r.mu.RUnlock()

	results := make([]Health, 0, len(snapshot))
	for _, c := range snapshot {
		results = append(results, c.Health(ctx))
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
