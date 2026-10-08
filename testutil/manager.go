package testutil

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/util"
)

// Manager provides lifecycle management for multiple test components. It allows starting, stopping,
// and resetting multiple components together, making it easier to manage complex test setups.
type Manager struct {
	ctx        context.Context
	budgets    Budgets
	components []component.Component
	started    []component.Component
	active     bool
	cleanupErr error
	opMu       sync.Mutex
	mu         sync.RWMutex
}

// NewManager creates a new test component manager.
func NewManager(ctx context.Context, opts ...Option) *Manager {
	return &Manager{
		ctx:        ctx,
		budgets:    ResolveBudgets(opts...),
		components: make([]component.Component, 0),
	}
}

// Add registers a test component with the manager.
func (m *Manager) Add(comp component.Component) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		return fmt.Errorf("cannot register components during an active lifecycle")
	}
	if util.IsNil(comp) {
		return fmt.Errorf("test component is required")
	}
	for _, existing := range m.components {
		if existing.Name() == comp.Name() {
			return fmt.Errorf("duplicate test component %q", comp.Name())
		}
	}
	m.components = append(m.components, comp)
	return nil
}

// Components returns all registered components.
func (m *Manager) Components() []component.Component {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]component.Component, len(m.components))
	copy(result, m.components)
	return result
}

// Get retrieves a component by name. Returns nil if no component with the given name is found.
func (m *Manager) Get(name string) component.Component {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, comp := range m.components {
		if comp.Name() == name {
			return comp
		}
	}
	return nil
}

// StartAll starts all registered components in order. If any component fails to start,
// unwinds successful starts in reverse order using a fresh cleanup budget. A component whose Start fails must release its own partial acquisition.
func (m *Manager) StartAll() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	if m.active {
		m.mu.Unlock()
		return fmt.Errorf("test environment already started")
	}
	m.active = true
	m.cleanupErr = nil
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(m.ctx, m.budgets.Setup)
	defer cancel()
	for _, comp := range m.Components() {
		err := ctx.Err()
		if err == nil {
			err = comp.Start(ctx)
		}
		if err != nil {
			return errors.Join(fmt.Errorf("failed to start component %s: %w", comp.Name(), err), m.stopAll())
		}
		m.started = append(m.started, comp)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(err, m.stopAll())
	}
	return nil
}

// StopAll stops all registered components in reverse order. Even if some components fail to stop,
// continues stopping others and returns a combined error with all failures.
func (m *Manager) StopAll() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.stopAll()
}

func (m *Manager) stopAll() error {
	if len(m.started) == 0 {
		m.mu.Lock()
		m.active = false
		m.mu.Unlock()
		return m.cleanupErr
	}
	ctx, cancel := cleanupContext(m.ctx, m.budgets.Cleanup)
	defer cancel()
	var errs []error

	// Stop in reverse order (LIFO)
	for i := len(m.started) - 1; i >= 0; i-- {
		comp := m.started[i]
		deadline, _ := ctx.Deadline()
		stopCtx, stopCancel := context.WithTimeout(ctx, time.Until(deadline)/time.Duration(i+1))
		err := errors.Join(comp.Stop(stopCtx), stopCtx.Err())
		stopCancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to stop component %s: %w", comp.Name(), err))
		}
	}

	m.started = nil
	m.cleanupErr = errors.Join(errs...)
	m.mu.Lock()
	m.active = false
	m.mu.Unlock()
	return m.cleanupErr
}

// ResetAll resets all registered components to their initial state.
// If any component fails to reset, returns immediately with that error.
func (m *Manager) ResetAll() error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	ctx, cancel := context.WithTimeout(m.ctx, m.budgets.Setup)
	defer cancel()

	for _, comp := range m.Components() {
		resetter, ok := comp.(interface{ Reset(context.Context) error })
		if !ok {
			return fmt.Errorf("component %s does not support reset: %w", comp.Name(), errors.ErrUnsupported)
		}
		if err := resetter.Reset(ctx); err != nil {
			return fmt.Errorf("failed to reset component %s: %w", comp.Name(), err)
		}
	}
	return nil
}

// Cleanup is an alias for StopAll, provided for convenience. This makes it easy to use with defer
// or testing.T.Cleanup().
func (m *Manager) Cleanup() error {
	return m.StopAll()
}
