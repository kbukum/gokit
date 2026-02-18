# Database TestUtil

Testing utilities for the `database` module, providing an in-memory SQLite test database component and fixture helpers.

## Features

- **TestComponent**: In-memory SQLite database with full TestComponent lifecycle support
- **State Management**: Reset, Snapshot, and Restore capabilities for test isolation
- **Fixture Helpers**: Load test data, truncate tables, assert row counts
- **Auto-Migration**: Optional model auto-migration on startup
- **Zero Configuration**: Works out of the box with sensible defaults

## Quick Start

### Basic Usage

```go
package mypackage_test

import (
    "testing"
    dbtestutil "github.com/kbukum/gokit/database/testutil"
    "github.com/kbukum/gokit/testutil"
)

func TestMyFeature(t *testing.T) {
    // Create and start test database
    db := dbtestutil.NewComponent()
    testutil.T(t).Setup(db)
    
    // Use the database
    db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
    db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Alice")
    
    // Your test code here...
}
```

### With Auto-Migration

```go
func TestWithModels(t *testing.T) {
    type User struct {
        ID   uint   `gorm:"primarykey"`
        Name string
    }
    
    // Component will auto-migrate the User model on Start()
    db := dbtestutil.NewComponent().WithModels(&User{})
    testutil.T(t).Setup(db)
    
    // Table is ready to use
    db.DB().Create(&User{Name: "Alice"})
}
```

## TestComponent Interface

The database `Component` implements `testutil.TestComponent`:

- **Reset()**: Clears all data from all tables while preserving schema
- **Snapshot()**: Captures current database state (all tables and rows)
- **Restore(snapshot)**: Restores database to a previous snapshot

### State Management Example

```go
func TestWithStateManagement(t *testing.T) {
    db := dbtestutil.NewComponent()
    testutil.T(t).Setup(db)
    
    db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
    db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Alice")
    
    // Capture state
    snapshot := testutil.T(t).Snapshot(db)
    
    // Modify data
    db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Bob")
    
    // Restore to snapshot - Bob is gone, only Alice remains
    testutil.T(t).Restore(db, snapshot)
    
    // Reset completely - all data cleared
    testutil.T(t).Reset(db)
}
```

## Fixture Helpers

Convenient functions for managing test data:

### Loading Data

```go
func TestWithFixtures(t *testing.T) {
    db := dbtestutil.NewComponent()
    testutil.T(t).Setup(db)
    
    db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)")
    
    // Load fixture data
    dbtestutil.LoadFixture(db.DB(), "users", []map[string]interface{}{
        {"name": "Alice", "email": "alice@example.com"},
        {"name": "Bob", "email": "bob@example.com"},
    })
    
    // Or use MustLoadFixture to fail test on error
    dbtestutil.MustLoadFixture(t, db.DB(), "users", []map[string]interface{}{
        {"name": "Charlie", "email": "charlie@example.com"},
    })
}
```

### Table Operations

```go
// Check if table exists
if dbtestutil.TableExists(db.DB(), "users") {
    // ...
}

// Get all table names
tables, err := dbtestutil.GetTableNames(db.DB())

// Count rows
count, err := dbtestutil.CountRows(db.DB(), "users")

// Truncate a table
dbtestutil.TruncateTable(db.DB(), "users")

// Truncate all tables
dbtestutil.TruncateAllTables(db.DB())
```

### Assertions

```go
func TestAssertions(t *testing.T) {
    db := dbtestutil.NewComponent()
    testutil.T(t).Setup(db)
    
    db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY)")
    
    // Assert table is empty
    dbtestutil.AssertTableEmpty(t, db.DB(), "users")
    
    db.DB().Exec("INSERT INTO users (id) VALUES (1)")
    db.DB().Exec("INSERT INTO users (id) VALUES (2)")
    
    // Assert specific row count
    dbtestutil.AssertRowCount(t, db.DB(), "users", 2)
}
```

## Table-Driven Tests with Reset

Use `Reset()` to ensure test isolation in table-driven tests:

```go
func TestUserCRUD(t *testing.T) {
    db := dbtestutil.NewComponent().WithModels(&User{})
    testutil.T(t).Setup(db)
    
    tests := []struct {
        name string
        fn   func(t *testing.T)
    }{
        {"Create", testCreateUser},
        {"Read", testReadUser},
        {"Update", testUpdateUser},
        {"Delete", testDeleteUser},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Reset to clean state before each test case
            testutil.T(t).Reset(db)
            tt.fn(t)
        })
    }
}
```

## Multiple Snapshots

You can take multiple snapshots and restore to any of them:

```go
func TestMultipleStates(t *testing.T) {
    db := dbtestutil.NewComponent()
    testutil.T(t).Setup(db)
    
    db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
    
    // State 1: Empty
    snap1 := testutil.T(t).Snapshot(db)
    
    // State 2: One user
    db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Alice")
    snap2 := testutil.T(t).Snapshot(db)
    
    // State 3: Two users
    db.DB().Exec("INSERT INTO users (name) VALUES (?)", "Bob")
    snap3 := testutil.T(t).Snapshot(db)
    
    // Jump back to any state
    testutil.T(t).Restore(db, snap2)  // Back to one user
    testutil.T(t).Restore(db, snap1)  // Back to empty
    testutil.T(t).Restore(db, snap3)  // Forward to two users
}
```

## Integration Tests

Combine with other test components for integration testing:

```go
func TestIntegration(t *testing.T) {
    ctx := context.Background()
    manager := testutil.NewManager(ctx)
    
    // Add components
    db := dbtestutil.NewComponent().WithModels(&User{})
    redis := redistestutil.NewComponent()  // from redis/testutil
    
    manager.Add(db)
    manager.Add(redis)
    
    // Start all components
    if err := manager.StartAll(); err != nil {
        t.Fatal(err)
    }
    defer manager.Cleanup()
    
    // Run integration tests...
}
```

## Best Practices

### 1. Use Auto-Migration for Models

When testing with GORM models, use `WithModels()` to automatically create tables:

```go
// Good ✓
db := dbtestutil.NewComponent().WithModels(&User{}, &Post{})
testutil.T(t).Setup(db)

// Avoid ✗ - manual table creation when using GORM models
db.DB().Exec("CREATE TABLE users ...")
```

### 2. Reset Between Test Cases

Always reset state between test cases to ensure isolation:

```go
// Good ✓
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        testutil.T(t).Reset(db)
        tt.fn(t)
    })
}

// Avoid ✗ - tests may interfere with each other
for _, tt := range tests {
    t.Run(tt.name, tt.fn)
}
```

### 3. Use Snapshots for Complex State

When you need to return to the same complex state multiple times:

```go
// Good ✓
testutil.T(t).Setup(db)
// ... setup complex test data ...
snapshot := testutil.T(t).Snapshot(db)

// Test case 1
// ... modify data ...
testutil.T(t).Restore(db, snapshot)

// Test case 2 - starts from same state
// ... modify data ...
testutil.T(t).Restore(db, snapshot)
```

### 4. Use Fixture Helpers

Prefer fixture helpers over raw SQL when loading test data:

```go
// Good ✓
dbtestutil.MustLoadFixture(t, db.DB(), "users", []map[string]interface{}{
    {"name": "Alice", "email": "alice@example.com"},
    {"name": "Bob", "email": "bob@example.com"},
})

// Acceptable but more verbose ✗
db.DB().Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Alice", "alice@example.com")
db.DB().Exec("INSERT INTO users (name, email) VALUES (?, ?)", "Bob", "bob@example.com")
```

### 5. Use Assertions for Validation

Use assertion helpers to make tests more readable:

```go
// Good ✓
dbtestutil.AssertRowCount(t, db.DB(), "users", 5)

// Avoid ✗ - manual count and assertion
var count int64
db.DB().Raw("SELECT COUNT(*) FROM users").Scan(&count)
if count != 5 {
    t.Errorf("count = %d, want 5", count)
}
```

## API Reference

### Component

```go
// NewComponent creates a new test database component
func NewComponent() *Component

// WithModels registers models for auto-migration
func (c *Component) WithModels(models ...interface{}) *Component

// DB returns the underlying *gorm.DB
func (c *Component) DB() *gorm.DB

// Component interface methods
Name() string
Start(ctx context.Context) error
Stop(ctx context.Context) error
Health(ctx context.Context) component.Health

// TestComponent interface methods
Reset(ctx context.Context) error
Snapshot(ctx context.Context) (interface{}, error)
Restore(ctx context.Context, snapshot interface{}) error
```

### Fixture Functions

```go
// LoadFixture loads test data into a table
func LoadFixture(db *gorm.DB, table string, data []map[string]interface{}) error

// MustLoadFixture loads test data and fails the test on error
func MustLoadFixture(t *testing.T, db *gorm.DB, table string, data []map[string]interface{})

// TruncateTable removes all rows from a table
func TruncateTable(db *gorm.DB, table string) error

// TruncateAllTables removes all rows from all tables
func TruncateAllTables(db *gorm.DB) error

// TableExists checks if a table exists
func TableExists(db *gorm.DB, table string) bool

// GetTableNames returns a list of all non-system tables
func GetTableNames(db *gorm.DB) ([]string, error)

// CountRows returns the number of rows in a table
func CountRows(db *gorm.DB, table string) (int64, error)

// AssertTableEmpty fails the test if the table is not empty
func AssertTableEmpty(t *testing.T, db *gorm.DB, table string)

// AssertRowCount fails the test if the table doesn't have the expected row count
func AssertRowCount(t *testing.T, db *gorm.DB, table string, expected int64)
```

## Implementation Notes

- Uses SQLite in-memory database (`:memory:`)
- Thread-safe with mutex-protected operations
- Automatically handles table schema preservation during Reset
- Snapshot captures all tables and their data
- Compatible with GORM models and raw SQL

## Examples

See the test files for comprehensive examples:
- `component_test.go` - TestComponent lifecycle and state management
- `fixtures_test.go` - Fixture helper usage patterns

## Coverage

Current test coverage: **84.2%** (32 tests, all passing)
