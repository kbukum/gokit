package testutil

import (
	"context"
	"testing"
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
func Setup(component TestComponent) (CleanupFunc, error) {
	return SetupWithContext(context.Background(), component)
}

// SetupWithContext starts a test component with a custom context and returns a cleanup function.
func SetupWithContext(ctx context.Context, component TestComponent) (CleanupFunc, error) {
	if err := component.Start(ctx); err != nil {
		return nil, err
	}

	cleanup := func() error {
		return component.Stop(ctx)
	}

	return cleanup, nil
}

// Teardown stops a test component.
// This is the inverse of Setup and is provided for symmetry.
func Teardown(component TestComponent) error {
	return TeardownWithContext(context.Background(), component)
}

// TeardownWithContext stops a test component with a custom context.
func TeardownWithContext(ctx context.Context, component TestComponent) error {
	return component.Stop(ctx)
}

// ResetComponent resets a test component to its initial state.
func ResetComponent(component TestComponent) error {
	return ResetComponentWithContext(context.Background(), component)
}

// ResetComponentWithContext resets a test component with a custom context.
func ResetComponentWithContext(ctx context.Context, component TestComponent) error {
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
		ctx: context.Background(),
	}
}

// WithContext sets a custom context for the helper.
func (h *THelper) WithContext(ctx context.Context) *THelper {
	h.ctx = ctx
	return h
}

// Setup starts a component and registers cleanup with testing.T.
// The component will be automatically stopped when the test ends.
func (h *THelper) Setup(component TestComponent) {
	if err := component.Start(h.ctx); err != nil {
		h.t.Fatalf("failed to start component %s: %v", component.Name(), err)
	}

	h.t.Cleanup(func() {
		if err := component.Stop(h.ctx); err != nil {
			h.t.Errorf("failed to stop component %s: %v", component.Name(), err)
		}
	})
}

// Reset resets a component to its initial state.
func (h *THelper) Reset(component TestComponent) {
	if err := component.Reset(h.ctx); err != nil {
		h.t.Fatalf("failed to reset component %s: %v", component.Name(), err)
	}
}

// Snapshot captures the current state of a component.
func (h *THelper) Snapshot(component TestComponent) interface{} {
	snapshot, err := component.Snapshot(h.ctx)
	if err != nil {
		h.t.Fatalf("failed to snapshot component %s: %v", component.Name(), err)
	}
	return snapshot
}

// Restore restores a component to a previously captured state.
func (h *THelper) Restore(component TestComponent, snapshot interface{}) {
	if err := component.Restore(h.ctx, snapshot); err != nil {
		h.t.Fatalf("failed to restore component %s: %v", component.Name(), err)
	}
}
