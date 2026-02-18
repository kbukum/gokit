package testutil

import (
	"context"

	"github.com/kbukum/gokit/component"
)

// TestComponent extends component.Component with testing-specific lifecycle methods.
// Test components can be used both as regular components in the component registry
// and as test helpers with additional Reset/Snapshot/Restore capabilities.
type TestComponent interface {
	component.Component

	// Reset restores the component to its initial state.
	// This is typically used between test cases to ensure test isolation.
	Reset(ctx context.Context) error

	// Snapshot captures the current state of the component.
	// The returned data can be passed to Restore() to return to this state.
	Snapshot(ctx context.Context) (interface{}, error)

	// Restore restores the component to a previously captured state.
	// The snapshot parameter should be a value returned by Snapshot().
	Restore(ctx context.Context, snapshot interface{}) error
}
