// Package testutil provides testing utilities for the database module.
//
// It includes an in-memory SQLite test component that implements both component.Component
// and testutil.TestComponent interfaces, along with fixture helpers for loading test data
// and managing database state.
//
// # Quick Start
//
// Create a test database with automatic cleanup:
//
//	db := testutil.NewComponent()
//	testutil.T(t).Setup(db)
//
//	// Use db.DB() to access *gorm.DB
//	db.DB().Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
//
// # Auto-Migration
//
// Register models for automatic migration on Start():
//
//	type User struct {
//	    ID   uint   `gorm:"primarykey"`
//	    Name string
//	}
//
//	db := testutil.NewComponent().WithModels(&User{})
//	testutil.T(t).Setup(db)
//
// # State Management
//
// Use Reset, Snapshot, and Restore for test isolation:
//
//	// Reset clears all data
//	testutil.T(t).Reset(db)
//
//	// Snapshot captures current state
//	snapshot := testutil.T(t).Snapshot(db)
//
//	// Restore returns to snapshot
//	testutil.T(t).Restore(db, snapshot)
//
// # Fixture Helpers
//
// Load test data easily:
//
//	MustLoadFixture(t, db.DB(), "users", []map[string]any{
//	    {"name": "Alice", "email": "alice@example.com"},
//	    {"name": "Bob", "email": "bob@example.com"},
//	})
//
//	AssertRowCount(t, db.DB(), "users", 2)
//
// See the README for more examples and best practices.
//
// # Migration Driver
//
// MigrationDriver is an in-memory golang-migrate driver fake for exercising migration
// orchestration (Up/Down/Steps/Reset/Version) without a real database backend. It records the
// applied version and run count and can be told to fail specific operations, so failure and
// rollback paths are provable deterministically:
//
//	driver := testutil.NewMigrationDriver()
//	cfg := migration.Config{DB: db, FS: fs, Path: "migrations", Driver: driver.DriverFunc()}
//	if err := cfg.Up(); err != nil { ... }
//
//	// Prove a failing rollback is surfaced, not swallowed:
//	driver.FailRun()
//	err := cfg.Down() // wrapped "migrate down" error
package testutil
