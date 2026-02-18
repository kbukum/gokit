package testutil

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
)

func TestComponent_Interface(t *testing.T) {
	tc := NewComponent()

	var _ component.Component = tc
	var _ testutil.TestComponent = tc
}

func TestComponent_Name(t *testing.T) {
	tc := NewComponent()

	if tc.Name() != "database-test" {
		t.Errorf("Name() = %q, want %q", tc.Name(), "database-test")
	}
}

func TestComponent_Lifecycle(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()

	// Start
	if err := tc.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// Health check
	health := tc.Health(ctx)
	if health.Status != component.StatusHealthy {
		t.Errorf("Health() status = %v, want %v", health.Status, component.StatusHealthy)
	}

	// Stop
	if err := tc.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestComponent_StartTwice(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()

	if err := tc.Start(ctx); err != nil {
		t.Fatalf("first Start() failed: %v", err)
	}
	defer tc.Stop(ctx)

	if err := tc.Start(ctx); err == nil {
		t.Error("second Start() should fail, got nil error")
	}
}

func TestComponent_StopBeforeStart(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()

	// Should be safe to stop without starting
	if err := tc.Stop(ctx); err != nil {
		t.Errorf("Stop() before Start() failed: %v", err)
	}
}

func TestComponent_HealthBeforeStart(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()

	health := tc.Health(ctx)
	if health.Status != component.StatusUnhealthy {
		t.Errorf("Health() before Start() status = %v, want %v", health.Status, component.StatusUnhealthy)
	}
}

func TestComponent_Reset(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()
	testutil.T(t).Setup(tc)

	db := tc.DB()
	if db == nil {
		t.Fatal("DB() returned nil after Start()")
	}

	// Create test table and insert data
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").Error; err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	if err := db.Exec("INSERT INTO users (name) VALUES (?)", "Alice").Error; err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	// Verify data exists
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM users").Scan(&count).Error; err != nil {
		t.Fatalf("SELECT COUNT failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Reset should clear all data
	if err := tc.Reset(ctx); err != nil {
		t.Fatalf("Reset() failed: %v", err)
	}

	// Table should exist but be empty
	if err := db.Raw("SELECT COUNT(*) FROM users").Scan(&count).Error; err != nil {
		t.Fatalf("SELECT COUNT after Reset failed: %v", err)
	}
	if count != 0 {
		t.Errorf("count after Reset = %d, want 0", count)
	}
}

func TestComponent_ResetBeforeStart(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()

	if err := tc.Reset(ctx); err == nil {
		t.Error("Reset() before Start() should fail, got nil error")
	}
}

func TestComponent_Snapshot(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()
	testutil.T(t).Setup(tc)

	db := tc.DB()

	// Create test data
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").Error; err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	if err := db.Exec("INSERT INTO users (name) VALUES (?)", "Alice").Error; err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	// Take snapshot
	snapshot, err := tc.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Snapshot() returned nil")
	}

	// Modify data
	if err := db.Exec("INSERT INTO users (name) VALUES (?)", "Bob").Error; err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM users").Scan(&count).Error; err != nil {
		t.Fatalf("SELECT COUNT failed: %v", err)
	}
	if count != 2 {
		t.Errorf("count before restore = %d, want 2", count)
	}
}

func TestComponent_SnapshotBeforeStart(t *testing.T) {
	tc := NewComponent()

	_, err := tc.Snapshot(context.Background())
	if err == nil {
		t.Error("Snapshot() before Start() should fail, got nil error")
	}
}

func TestComponent_Restore(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()
	testutil.T(t).Setup(tc)

	db := tc.DB()

	// Create test data
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").Error; err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	if err := db.Exec("INSERT INTO users (name) VALUES (?)", "Alice").Error; err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	// Take snapshot
	snapshot, err := tc.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() failed: %v", err)
	}

	// Modify data
	if err := db.Exec("INSERT INTO users (name) VALUES (?)", "Bob").Error; err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}
	if err := db.Exec("INSERT INTO users (name) VALUES (?)", "Charlie").Error; err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	// Restore to snapshot
	if err := tc.Restore(ctx, snapshot); err != nil {
		t.Fatalf("Restore() failed: %v", err)
	}

	// Should have only 1 row (Alice)
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM users").Scan(&count).Error; err != nil {
		t.Fatalf("SELECT COUNT failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count after restore = %d, want 1", count)
	}

	var name string
	if err := db.Raw("SELECT name FROM users").Scan(&name).Error; err != nil {
		t.Fatalf("SELECT name failed: %v", err)
	}
	if name != "Alice" {
		t.Errorf("name after restore = %q, want %q", name, "Alice")
	}
}

func TestComponent_RestoreBeforeStart(t *testing.T) {
	tc := NewComponent()

	if err := tc.Restore(context.Background(), map[string]interface{}{}); err == nil {
		t.Error("Restore() before Start() should fail, got nil error")
	}
}

func TestComponent_RestoreInvalidSnapshot(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()
	testutil.T(t).Setup(tc)

	if err := tc.Restore(ctx, "invalid"); err == nil {
		t.Error("Restore() with invalid snapshot should fail, got nil error")
	}
}

func TestComponent_DB(t *testing.T) {
	tc := NewComponent()

	// Before start
	if db := tc.DB(); db != nil {
		t.Error("DB() before Start() should return nil")
	}

	// After start
	testutil.T(t).Setup(tc)
	if db := tc.DB(); db == nil {
		t.Error("DB() after Start() should not return nil")
	}
}

func TestComponent_WithModels(t *testing.T) {
	type User struct {
		ID   uint `gorm:"primarykey"`
		Name string
	}

	tc := NewComponent().WithModels(&User{})
	testutil.T(t).Setup(tc)

	db := tc.DB()

	// Table should be auto-migrated
	if !db.Migrator().HasTable(&User{}) {
		t.Error("WithModels() did not auto-migrate table")
	}

	// Should be able to use the table
	if err := db.Create(&User{Name: "Alice"}).Error; err != nil {
		t.Errorf("Create() failed: %v", err)
	}

	var count int64
	if err := db.Model(&User{}).Count(&count).Error; err != nil {
		t.Fatalf("Count() failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestComponent_MultipleSnapshots(t *testing.T) {
	ctx := context.Background()
	tc := NewComponent()
	testutil.T(t).Setup(tc)

	db := tc.DB()

	// Create test table
	if err := db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)").Error; err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	// State 1: Empty
	snap1, err := tc.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot 1 failed: %v", err)
	}

	// State 2: One user
	db.Exec("INSERT INTO users (name) VALUES (?)", "Alice")
	snap2, err := tc.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot 2 failed: %v", err)
	}

	// State 3: Two users
	db.Exec("INSERT INTO users (name) VALUES (?)", "Bob")
	snap3, err := tc.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot 3 failed: %v", err)
	}

	// Restore to state 2
	if err := tc.Restore(ctx, snap2); err != nil {
		t.Fatalf("Restore to snap2 failed: %v", err)
	}

	var count int64
	db.Raw("SELECT COUNT(*) FROM users").Scan(&count)
	if count != 1 {
		t.Errorf("after restore to snap2: count = %d, want 1", count)
	}

	// Restore to state 3
	if err := tc.Restore(ctx, snap3); err != nil {
		t.Fatalf("Restore to snap3 failed: %v", err)
	}

	db.Raw("SELECT COUNT(*) FROM users").Scan(&count)
	if count != 2 {
		t.Errorf("after restore to snap3: count = %d, want 2", count)
	}

	// Restore to state 1 (empty)
	if err := tc.Restore(ctx, snap1); err != nil {
		t.Fatalf("Restore to snap1 failed: %v", err)
	}

	db.Raw("SELECT COUNT(*) FROM users").Scan(&count)
	if count != 0 {
		t.Errorf("after restore to snap1: count = %d, want 0", count)
	}
}
