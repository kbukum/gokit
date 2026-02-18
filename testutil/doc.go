// Package testutil provides testing infrastructure for gokit components.
//
// The testutil package extends gokit's component lifecycle pattern with
// testing-specific capabilities, enabling easy setup, teardown, and state
// management for test components.
//
// # Quick Start
//
// Basic usage with automatic cleanup:
//
//	func TestMyFeature(t *testing.T) {
//	    testutil.T(t).Setup(myComponent)
//	    // Component is automatically cleaned up when test ends
//	}
//
// Manual cleanup:
//
//	cleanup, err := testutil.Setup(myComponent)
//	if err != nil {
//	    t.Fatal(err)
//	}
//	defer cleanup()
//
// Managing multiple components:
//
//	manager := testutil.NewManager(ctx)
//	manager.Add(dbComponent)
//	manager.Add(redisComponent)
//	manager.StartAll()
//	defer manager.Cleanup()
//
// # Architecture
//
// The TestComponent interface extends component.Component with three
// testing-specific methods:
//
//   - Reset(ctx): Restore component to initial state
//   - Snapshot(ctx): Capture current state
//   - Restore(ctx, snapshot): Restore to a captured state
//
// This hybrid approach provides consistency with production code while
// adding testing capabilities needed for test isolation and state management.
//
// # Thread Safety
//
// All Manager operations are thread-safe. Individual TestComponent
// implementations should ensure thread-safety if used in concurrent tests.
//
// See the README.md for comprehensive documentation and examples.
package testutil
