// Package testutil provides testing utilities for the database module.
package testutil

import (
	"context"
	"fmt"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
)

// Component is a test database component that uses SQLite in-memory.
// It implements both component.Component and testutil.TestComponent interfaces.
type Component struct {
	db      *gorm.DB
	models  []interface{}
	started bool
	mu      sync.RWMutex
}

// Ensure Component implements the required interfaces
var _ component.Component = (*Component)(nil)
var _ testutil.TestComponent = (*Component)(nil)

// NewComponent creates a new test database component.
// By default, it uses SQLite in-memory database.
func NewComponent() *Component {
	return &Component{}
}

// WithModels registers models for auto-migration on Start.
// This is useful when you want the component to automatically create
// tables for your models during component startup.
func (c *Component) WithModels(models ...interface{}) *Component {
	c.models = append(c.models, models...)
	return c
}

// DB returns the underlying *gorm.DB, or nil if not started.
func (c *Component) DB() *gorm.DB {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.db
}

// Name returns the component name.
func (c *Component) Name() string {
	return "database-test"
}

// Start initializes the in-memory SQLite database.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("component already started")
	}

	// Open in-memory SQLite database
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to open test database: %w", err)
	}

	c.db = db
	c.started = true

	// Auto-migrate models if any were registered
	if len(c.models) > 0 {
		if err := c.db.AutoMigrate(c.models...); err != nil {
			return fmt.Errorf("auto-migrate failed: %w", err)
		}
	}

	return nil
}

// Stop closes the database connection.
func (c *Component) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started || c.db == nil {
		return nil
	}

	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}

	c.started = false
	return sqlDB.Close()
}

// Health returns the health status of the test database.
func (c *Component) Health(ctx context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.db == nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "database not started",
		}
	}

	sqlDB, err := c.db.DB()
	if err != nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("failed to get sql.DB: %v", err),
		}
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("ping failed: %v", err),
		}
	}

	return component.Health{
		Name:   c.Name(),
		Status: component.StatusHealthy,
	}
}

// Reset clears all data from all tables while preserving the schema.
// This is useful for resetting state between test cases.
func (c *Component) Reset(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.db == nil {
		return fmt.Errorf("component not started")
	}

	// Get all table names
	var tables []string
	if err := c.db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").
		Scan(&tables).Error; err != nil {
		return fmt.Errorf("failed to list tables: %w", err)
	}

	// Delete all rows from each table
	for _, table := range tables {
		if err := c.db.Exec(fmt.Sprintf("DELETE FROM %s", table)).Error; err != nil {
			return fmt.Errorf("failed to clear table %s: %w", table, err)
		}
	}

	return nil
}

// Snapshot captures the current state of the database.
// Returns a snapshot that can be used with Restore to return to this state.
func (c *Component) Snapshot(ctx context.Context) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.db == nil {
		return nil, fmt.Errorf("component not started")
	}

	snapshot := make(map[string][]map[string]interface{})

	// Get all table names
	var tables []string
	if err := c.db.Raw("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").
		Scan(&tables).Error; err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	// For each table, capture all rows
	for _, table := range tables {
		var rows []map[string]interface{}
		if err := c.db.Raw(fmt.Sprintf("SELECT * FROM %s", table)).
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("failed to snapshot table %s: %w", table, err)
		}
		snapshot[table] = rows
	}

	return snapshot, nil
}

// Restore returns the database to a previously captured snapshot state.
// The snapshot must have been created by the Snapshot method.
func (c *Component) Restore(ctx context.Context, snap interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.db == nil {
		return fmt.Errorf("component not started")
	}

	snapshot, ok := snap.(map[string][]map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid snapshot type: expected map[string][]map[string]interface{}, got %T", snap)
	}

	// First, clear all tables
	if err := c.Reset(ctx); err != nil {
		return fmt.Errorf("failed to reset before restore: %w", err)
	}

	// Restore data for each table
	for table, rows := range snapshot {
		for _, row := range rows {
			if err := c.db.Table(table).Create(row).Error; err != nil {
				return fmt.Errorf("failed to restore row to table %s: %w", table, err)
			}
		}
	}

	return nil
}
