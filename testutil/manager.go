package testutil

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Manager provides lifecycle management for multiple test components.
// It allows starting, stopping, and resetting multiple components together,
// making it easier to manage complex test setups.
type Manager struct {
	ctx        context.Context
	components []TestComponent
	mu         sync.RWMutex
}

// NewManager creates a new test component manager.
func NewManager(ctx context.Context) *Manager {
	return &Manager{
		ctx:        ctx,
		components: make([]TestComponent, 0),
	}
}

// Add registers a test component with the manager.
func (m *Manager) Add(component TestComponent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components = append(m.components, component)
}

// Components returns all registered components.
func (m *Manager) Components() []TestComponent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]TestComponent, len(m.components))
	copy(result, m.components)
	return result
}

// Get retrieves a component by name.
// Returns nil if no component with the given name is found.
func (m *Manager) Get(name string) TestComponent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, comp := range m.components {
		if comp.Name() == name {
			return comp
		}
	}
	return nil
}

// StartAll starts all registered components in order.
// If any component fails to start, returns immediately with that error.
func (m *Manager) StartAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, comp := range m.components {
		if err := comp.Start(m.ctx); err != nil {
			return fmt.Errorf("failed to start component %s: %w", comp.Name(), err)
		}
	}
	return nil
}

// StopAll stops all registered components in reverse order.
// Even if some components fail to stop, continues stopping others and
// returns a combined error with all failures.
func (m *Manager) StopAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var errs []error
	
	// Stop in reverse order (LIFO)
	for i := len(m.components) - 1; i >= 0; i-- {
		comp := m.components[i]
		if err := comp.Stop(m.ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop component %s: %w", comp.Name(), err))
		}
	}
	
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// ResetAll resets all registered components to their initial state.
// If any component fails to reset, returns immediately with that error.
func (m *Manager) ResetAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	for _, comp := range m.components {
		if err := comp.Reset(m.ctx); err != nil {
			return fmt.Errorf("failed to reset component %s: %w", comp.Name(), err)
		}
	}
	return nil
}

// Cleanup is an alias for StopAll, provided for convenience.
// This makes it easy to use with defer or testing.T.Cleanup().
func (m *Manager) Cleanup() error {
	return m.StopAll()
}
