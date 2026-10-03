package testutil_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
)

// ExampleSetup demonstrates basic component setup and teardown
func ExampleSetup() {
	// Create a test component
	comp := newMockComponent("my-service")

	// Setup starts the component and returns cleanup function
	cleanup, err := testutil.Setup(comp)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	// Use the component in your test
	health := comp.Health(context.Background())
	fmt.Println(health.Status)

	// Output: healthy
}

func TestExampleTHelper(t *testing.T) {
	t.Parallel()
	// Setup with automatic cleanup
	comp := newMockComponent("my-service")
	testutil.T(t).Setup(comp)

	// Component is started and will be automatically cleaned up when test ends
	health := comp.Health(context.Background())
	if health.Status != component.StatusHealthy {
		t.Fatalf("health = %s", health.Status)
	}
}

// ExampleManager demonstrates managing multiple components
func ExampleManager() {
	ctx := context.Background()

	// Create manager
	manager := testutil.NewManager(ctx)

	// Add multiple components
	manager.Add(newMockComponent("database"))
	manager.Add(newMockComponent("redis"))
	manager.Add(newMockComponent("kafka"))

	// Start all components
	if err := manager.StartAll(); err != nil {
		panic(err)
	}
	defer manager.Cleanup()

	// Get specific component
	db := manager.Get("database")
	fmt.Println(db.Name())

	// Output: database
}

// ExampleManager_resetAll demonstrates resetting all components
func ExampleManager_resetAll() {
	ctx := context.Background()
	manager := testutil.NewManager(ctx)

	comp := newMockComponent("test-db")
	manager.Add(comp)

	// Start component
	manager.StartAll()

	// Use component...
	// (modify state)

	// Reset all components to initial state
	if err := manager.ResetAll(); err != nil {
		panic(err)
	}

	fmt.Println("Components reset successfully")

	// Output: Components reset successfully
}

func TestExampleTHelperSnapshot(t *testing.T) {
	t.Parallel()
	comp := newMockComponent("stateful-service")

	testutil.T(t).Setup(comp)

	// Capture current state
	snapshot := testutil.T(t).Snapshot(comp)

	// ... modify component state ...

	// Restore to previous state
	testutil.T(t).Restore(comp, snapshot)

	if comp.restoreData == nil {
		t.Fatal("snapshot was not restored")
	}
}

// ExampleTestComponent demonstrates implementing a custom test component
func Example_customTestComponent() {
	// This example shows the interface that test components must implement
	type MyTestComponent struct {
		name    string
		started bool
	}

	// Component interface
	_ = func(c *MyTestComponent) string { return c.name }                       // Name()
	_ = func(c *MyTestComponent) error { c.started = true; return nil }         // Start(ctx)
	_ = func(c *MyTestComponent) error { c.started = false; return nil }        // Stop(ctx)
	_ = func(c *MyTestComponent) component.Health { return component.Health{} } // Health(ctx)

	// TestComponent interface additions
	_ = func(c *MyTestComponent) error { return nil }
	_ = func(c *MyTestComponent) (any, error) { return map[string]any{}, nil }
	_ = func(c *MyTestComponent) error { return nil }

	fmt.Println("TestComponent interface implemented")

	// Output: TestComponent interface implemented
}
