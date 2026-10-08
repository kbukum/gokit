package testutil

import (
	"context"
	"testing"
	"time"

	kitcomponent "github.com/kbukum/gokit/component"
)

// CleanupFunc is a function that performs cleanup, typically stopping a component.
type CleanupFunc func() error

// Setup starts a test component and returns a cleanup function.
// The cleanup function should be called (typically with defer) to stop the component.
//
// Example:
//
//	cleanup, err := testutil.Setup(dbComponent)
//	if err != nil {
//	    t.Fatal(err)
//	}
//	defer cleanup()
func Setup(comp kitcomponent.Component, opts ...Option) (CleanupFunc, error) {
	return SetupWithContext(context.Background(), comp, opts...)
}

// SetupWithContext starts a test component with a custom context and returns a cleanup function.
func SetupWithContext(ctx context.Context, comp kitcomponent.Component, opts ...Option) (CleanupFunc, error) {
	manager := NewManager(ctx, opts...)
	if err := manager.Add(comp); err != nil {
		return nil, err
	}
	if err := manager.StartAll(); err != nil { //nolint:contextcheck // Manager owns the supplied setup context and fresh cleanup contexts.
		return nil, err
	}
	return manager.Cleanup, nil
}

// Teardown stops a test component. This is the inverse of Setup and is provided for symmetry.
func Teardown(comp kitcomponent.Component) error {
	return TeardownWithContext(context.Background(), comp)
}

// TeardownWithContext stops a test component with a custom context.
func TeardownWithContext(ctx context.Context, comp kitcomponent.Component) error {
	ctx, cancel := cleanupContext(ctx, ResolveBudgets().Cleanup)
	defer cancel()
	return comp.Stop(ctx)
}

// ResetComponent resets a test component to its initial state.
func ResetComponent(component TestComponent) error {
	return ResetComponentWithContext(context.Background(), component)
}

// ResetComponentWithContext resets a test component with a custom context.
func ResetComponentWithContext(ctx context.Context, component TestComponent) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return component.Reset(ctx)
}

// THelper provides testing.T integration for easier test setup.
type THelper struct {
	t   *testing.T
	ctx context.Context
}

// T wraps a testing.T to provide helper methods.
// This integrates testutil with Go's testing package for automatic cleanup.
//
// Example:
//
//	func TestMyFeature(t *testing.T) {
//	    testutil.T(t).Setup(dbComponent)
//	    // component is automatically cleaned up when test ends
//	}
func T(t *testing.T) *THelper {
	return &THelper{
		t:   t,
		ctx: t.Context(),
	}
}

// WithContext sets a custom context for the helper.
func (h *THelper) WithContext(ctx context.Context) *THelper {
	h.ctx = ctx
	return h
}

// Setup starts a component and registers cleanup with testing.T.
// The component will be automatically stopped when the test ends.
func (h *THelper) Setup(comp kitcomponent.Component) {
	cleanup, err := SetupWithContext(h.ctx, comp)
	if err != nil {
		h.t.Fatalf("failed to start component %s: %v", comp.Name(), err)
	}

	h.t.Cleanup(func() {
		if err := cleanup(); err != nil {
			h.t.Errorf("failed to stop component %s: %v", comp.Name(), err)
		}
	})
}

// Reset resets a component to its initial state.
func (h *THelper) Reset(component TestComponent) {
	if err := ResetComponentWithContext(h.ctx, component); err != nil {
		h.t.Fatalf("failed to reset component %s: %v", component.Name(), err)
	}
}

// Snapshot captures the current state of a component.
func (h *THelper) Snapshot(component TestComponent) any {
	ctx, cancel := context.WithTimeout(h.ctx, 30*time.Second)
	defer cancel()
	snapshot, err := component.Snapshot(ctx)
	if err != nil {
		h.t.Fatalf("failed to snapshot component %s: %v", component.Name(), err)
	}
	return snapshot
}

// Restore restores a component to a previously captured state.
func (h *THelper) Restore(component TestComponent, snapshot any) {
	ctx, cancel := context.WithTimeout(h.ctx, 30*time.Second)
	defer cancel()
	if err := component.Restore(ctx, snapshot); err != nil {
		h.t.Fatalf("failed to restore component %s: %v", component.Name(), err)
	}
}
