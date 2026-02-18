# TestUtil - Testing Infrastructure for gokit

The `testutil` package provides a comprehensive testing infrastructure for gokit components, following the same lifecycle patterns as production components. It enables easy setup, teardown, and management of test components with support for state snapshots and resets.

## Features

- **TestComponent Interface**: Extends `component.Component` with testing-specific methods (Reset, Snapshot, Restore)
- **TestManager**: Lifecycle manager for coordinating multiple test components
- **Helper Functions**: Convenient wrappers for common testing patterns
- **Testing.T Integration**: Automatic cleanup integration with Go's testing package
- **Thread-Safe**: All operations are safe for concurrent use

## Quick Start

### Basic Usage

```go
package mypackage_test

import (
    "testing"
    "github.com/kbukum/gokit/testutil"
)

func TestMyFeature(t *testing.T) {
    // Option 1: Manual cleanup
    cleanup, err := testutil.Setup(myComponent)
    if err != nil {
        t.Fatal(err)
    }
    defer cleanup()
    
    // Your test code here...
}

func TestMyFeatureAutoCleanup(t *testing.T) {
    // Option 2: Automatic cleanup with testing.T
    testutil.T(t).Setup(myComponent)
    
    // Component is automatically cleaned up when test ends
    // Your test code here...
}
```

### Managing Multiple Components

```go
func TestIntegration(t *testing.T) {
    ctx := context.Background()
    manager := testutil.NewManager(ctx)
    
    // Add components
    manager.Add(databaseComponent)
    manager.Add(redisComponent)
    manager.Add(kafkaComponent)
    
    // Start all components
    if err := manager.StartAll(); err != nil {
        t.Fatal(err)
    }
    defer manager.Cleanup()
    
    // Your integration test here...
}
```

### State Management

```go
func TestWithStateReset(t *testing.T) {
    testutil.T(t).Setup(dbComponent)
    
    // Run first test case
    // ... modify database state ...
    
    // Reset to initial state
    testutil.T(t).Reset(dbComponent)
    
    // Run second test case with clean state
}

func TestWithSnapshotRestore(t *testing.T) {
    testutil.T(t).Setup(dbComponent)
    
    // Setup initial test data
    // ... populate database ...
    
    // Capture current state
    snapshot := testutil.T(t).Snapshot(dbComponent)
    
    // Run test that modifies state
    // ... modify database ...
    
    // Restore to snapshot
    testutil.T(t).Restore(dbComponent, snapshot)
    
    // State is back to the snapshot point
}
```

## Architecture

### TestComponent Interface

The `TestComponent` interface extends `component.Component` with testing-specific lifecycle methods:

```go
type TestComponent interface {
    component.Component  // Name(), Start(), Stop(), Health()
    
    // Testing-specific methods
    Reset(ctx context.Context) error
    Snapshot(ctx context.Context) (interface{}, error)
    Restore(ctx context.Context, snapshot interface{}) error
}
```

This hybrid approach provides:
- **Consistency**: Same lifecycle pattern as production components
- **Flexibility**: Can be used as both a Component and a test helper
- **Integration**: Works with existing component infrastructure (Registry, etc.)

### TestManager

The `Manager` coordinates lifecycle operations across multiple components:

- **StartAll()**: Starts all components in order
- **StopAll()**: Stops all components in reverse order (LIFO)
- **ResetAll()**: Resets all components to initial state
- **Get(name)**: Retrieve a specific component by name
- **Cleanup()**: Alias for StopAll() for defer usage

### Helper Functions

#### Setup/Teardown

```go
// Setup starts a component and returns cleanup function
cleanup, err := testutil.Setup(component)
defer cleanup()

// With custom context
cleanup, err := testutil.SetupWithContext(ctx, component)
defer cleanup()

// Teardown stops a component
err := testutil.Teardown(component)
err := testutil.TeardownWithContext(ctx, component)
```

#### Reset

```go
// Reset a component to initial state
err := testutil.ResetComponent(component)
err := testutil.ResetComponentWithContext(ctx, component)
```

#### Testing.T Integration

```go
// T() provides automatic cleanup integration
testutil.T(t).Setup(component)          // Auto-cleanup on test end
testutil.T(t).Reset(component)          // Reset to initial state
snapshot := testutil.T(t).Snapshot(component)    // Capture state
testutil.T(t).Restore(component, snapshot)       // Restore state

// With custom context
testutil.T(t).WithContext(ctx).Setup(component)
```

## Best Practices

### 1. Use Automatic Cleanup

Prefer `testutil.T(t).Setup()` over manual cleanup to ensure resources are always freed:

```go
// Good ✓
testutil.T(t).Setup(component)

// Also good ✓
cleanup, err := testutil.Setup(component)
if err != nil {
    t.Fatal(err)
}
defer cleanup()

// Avoid ✗ - easy to forget cleanup
component.Start(ctx)
// ... test code ...
component.Stop(ctx)  // might not run if test fails
```

### 2. Use Manager for Multiple Components

When testing with multiple components, use `Manager` to coordinate them:

```go
manager := testutil.NewManager(ctx)
manager.Add(db)
manager.Add(redis)
manager.Add(kafka)

if err := manager.StartAll(); err != nil {
    t.Fatal(err)
}
defer manager.Cleanup()
```

### 3. Reset Between Test Cases

Use `Reset()` to ensure test isolation within table-driven tests:

```go
func TestCases(t *testing.T) {
    testutil.T(t).Setup(dbComponent)
    
    tests := []struct {
        name string
        // ...
    }{
        // test cases...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            testutil.T(t).Reset(dbComponent)  // Clean state for each case
            // ... test logic ...
        })
    }
}
```

### 4. Use Snapshots for Complex State

When you need to return to a specific state multiple times:

```go
testutil.T(t).Setup(dbComponent)

// Setup complex test data
// ... populate database with fixtures ...

snapshot := testutil.T(t).Snapshot(dbComponent)

// Test case 1
// ... modify state ...
testutil.T(t).Restore(dbComponent, snapshot)

// Test case 2 - starts from same snapshot
// ... modify state differently ...
testutil.T(t).Restore(dbComponent, snapshot)
```

### 5. Follow LIFO Order

When manually managing cleanup, always cleanup in reverse order (LIFO):

```go
cleanup1, _ := testutil.Setup(component1)
cleanup2, _ := testutil.Setup(component2)
cleanup3, _ := testutil.Setup(component3)

// Cleanup in reverse order
defer cleanup3()
defer cleanup2()
defer cleanup1()

// Or use Manager which handles this automatically
```

## Creating Test Components

To create a test component for your module, implement the `TestComponent` interface:

```go
package mymodule

import (
    "context"
    "github.com/kbukum/gokit/component"
    "github.com/kbukum/gokit/testutil"
)

type TestMyComponent struct {
    name string
    // ... component state ...
}

func NewTestComponent(name string) testutil.TestComponent {
    return &TestMyComponent{name: name}
}

// Component interface methods
func (c *TestMyComponent) Name() string { return c.name }

func (c *TestMyComponent) Start(ctx context.Context) error {
    // Initialize component
    return nil
}

func (c *TestMyComponent) Stop(ctx context.Context) error {
    // Cleanup resources
    return nil
}

func (c *TestMyComponent) Health(ctx context.Context) component.Health {
    return component.Health{
        Name:   c.name,
        Status: component.StatusHealthy,
    }
}

// TestComponent interface methods
func (c *TestMyComponent) Reset(ctx context.Context) error {
    // Reset to initial state
    return nil
}

func (c *TestMyComponent) Snapshot(ctx context.Context) (interface{}, error) {
    // Capture current state
    return map[string]interface{}{
        "data": c.data,
    }, nil
}

func (c *TestMyComponent) Restore(ctx context.Context, snapshot interface{}) error {
    // Restore from snapshot
    state := snapshot.(map[string]interface{})
    c.data = state["data"]
    return nil
}
```

## Module Integration

Module-specific test utilities should be placed in a `testutil` subdirectory:

```
database/
├── testutil/
│   ├── component.go      # Database test component
│   └── memory.go         # In-memory implementation

kafka/
├── testutil/
│   ├── component.go      # Kafka test component
│   └── broker.go         # Mock broker

redis/
├── testutil/
│   ├── component.go      # Redis test component
│   └── mock.go           # Mock Redis
```

Each module testutil provides domain-specific test helpers while conforming to the `testutil.TestComponent` interface.

## Examples

See the test files for comprehensive examples:
- `component_test.go` - TestComponent interface tests
- `manager_test.go` - TestManager usage patterns
- `helpers_test.go` - Helper function examples

## Thread Safety

All TestManager operations are thread-safe and can be called concurrently. Individual TestComponent implementations should also ensure thread-safety if they will be used in concurrent tests.

## Next Steps

- See module-specific testutil packages for database, Redis, Kafka, etc.
- Review the [Testing Guide](../docs/testing-guide.md) for best practices (when available)
- Check the [TestUtil Cookbook](../docs/testutil-cookbook.md) for common patterns (when available)

## Contributing

When adding new test components:
1. Implement the `TestComponent` interface
2. Write comprehensive tests for the component
3. Document usage patterns in the component's README
4. Follow the existing naming conventions and patterns
