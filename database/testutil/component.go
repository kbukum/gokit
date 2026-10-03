// Package testutil provides testing utilities for the database module.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/database"
	"github.com/kbukum/gokit/testutil"
)

// Component is a test database component that uses SQLite in-memory.
// It implements both component.Component and testutil.TestComponent interfaces.
type Component struct {
	db         *gorm.DB
	models     []any
	started    bool
	open       func(context.Context) (*database.DB, error)
	initialize func(context.Context, *gorm.DB) error
	close      func() error
	stopErr    error
	mu         sync.RWMutex
}

// Ensure Component implements the required interfaces
var (
	_ component.Component    = (*Component)(nil)
	_ testutil.TestComponent = (*Component)(nil)
)

// NewComponent creates a new test database component. By default,
// it uses SQLite in-memory database.
func NewComponent() *Component {
	return &Component{}
}

// WithDatabase injects an owned production database opener. The opener must release partial acquisitions on failure; on success this component owns both production pools. Configure before Start.
func (c *Component) WithDatabase(open func(context.Context) (*database.DB, error)) *Component {
	c.open = open
	return c
}

// WithInitializer injects context-bound production migrations or fixture setup after opening. Configure before Start; use WithModels only for small AutoMigrate unit fixtures.
func (c *Component) WithInitializer(initialize func(context.Context, *gorm.DB) error) *Component {
	c.initialize = initialize
	return c
}

// WithModels registers models for auto-migration on Start.
// This is useful when you want the component to automatically create tables for your models during component startup.
func (c *Component) WithModels(models ...any) *Component {
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
func (c *Component) Start(ctx context.Context) (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("component already started")
	}

	ctx, cancel := operationContext(ctx)
	defer cancel()
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	c.stopErr = nil
	var db *gorm.DB
	if c.open != nil {
		owned, openErr := c.open(ctx)
		if openErr != nil {
			return openErr
		}
		if owned == nil || owned.GormDB == nil {
			return fmt.Errorf("database opener returned no database")
		}
		db, c.close = owned.GormDB, owned.Close
	} else {
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard, DisableAutomaticPing: true})
		if err != nil {
			return fmt.Errorf("failed to open test database: %w", err)
		}
		pool, poolErr := db.DB()
		if poolErr != nil {
			return poolErr
		}
		pool.SetMaxOpenConns(1)
		pool.SetMaxIdleConns(1)
		c.close = pool.Close
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, c.close())
			c.close = nil
		}
	}()
	if err := db.WithContext(ctx).Exec("SELECT 1").Error; err != nil {
		return err
	}

	// Auto-migrate models if any were registered
	if len(c.models) > 0 {
		if err := db.WithContext(ctx).AutoMigrate(c.models...); err != nil {
			return fmt.Errorf("auto-migrate failed: %w", err)
		}
	}
	if c.initialize != nil {
		if err := c.initialize(ctx, db); err != nil {
			return fmt.Errorf("initialize test database: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.db = db
	c.started = true
	return c.stopErr
}

// Stop closes the database connection.
func (c *Component) Stop(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started || c.db == nil {
		return nil
	}

	c.started = false
	c.db = nil
	c.stopErr = c.close()
	c.close = nil
	return c.stopErr
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

	return TruncateAllTables(ctx, c.db)
}

// Snapshot captures the current state of the database.
// Returns a snapshot that can be used with Restore to return to this state.
func (c *Component) Snapshot(ctx context.Context) (any, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.db == nil {
		return nil, fmt.Errorf("component not started")
	}

	return c.snapshot(ctx)
}

// Restore returns the database to a previously captured snapshot state.
// The snapshot must have been created by the Snapshot method.
func (c *Component) Restore(ctx context.Context, snap any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.db == nil {
		return fmt.Errorf("component not started")
	}

	snapshot, ok := snap.(*databaseSnapshot)
	if !ok {
		return fmt.Errorf("invalid snapshot type: %T", snap)
	}

	return c.restore(ctx, snapshot)
}
