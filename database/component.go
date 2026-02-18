package database

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DriverFunc is a factory function that creates a GORM dialector.
// Standard GORM drivers (postgres.Open, mysql.Open, sqlite.Open) all match this signature.
type DriverFunc func(dsn string) gorm.Dialector

// Component wraps DB and implements component.Component for lifecycle management.
type Component struct {
	db         *DB
	cfg        Config
	log        *logger.Logger
	models     []interface{}
	driverFunc DriverFunc
}

// NewComponent creates a database component for use with the component registry.
func NewComponent(cfg Config, log *logger.Logger) *Component {
	return &Component{
		cfg: cfg,
		log: log.WithComponent("database"),
	}
}

// WithDriver sets the database driver function.
// Pass the Open function from your chosen driver (not the result of calling it).
//
// Example:
//
// import "gorm.io/driver/postgres"
// db := database.NewComponent(cfg, log).
//
//	WithDriver(postgres.Open).
//	WithAutoMigrate(&User{}, &Post{})
//
// If not set, SQLite is used as the default driver (useful for tests).
func (c *Component) WithDriver(fn DriverFunc) *Component {
	c.driverFunc = fn
	return c
}

// WithAutoMigrate registers models for auto-migration on Start.
func (c *Component) WithAutoMigrate(models ...interface{}) *Component {
	c.models = append(c.models, models...)
	return c
}

// DB returns the underlying *DB, or nil if not started.
func (c *Component) DB() *DB {
	return c.db
}

// ensure Component satisfies component.Component
var _ component.Component = (*Component)(nil)

// Name returns the component name.
func (c *Component) Name() string { return "database" }

// Start connects to the database and optionally runs auto-migration.
func (c *Component) Start(ctx context.Context) error {
	var dialectorFactory = sqlite.Open

	if c.driverFunc != nil {
		dialectorFactory = c.driverFunc
	}

	dialector := dialectorFactory(c.cfg.DSN)

	// Create connection using the dialector
	db, err := NewWithDialector(dialector, c.cfg, c.log)
	if err != nil {
		return fmt.Errorf("database start: %w", err)
	}
	c.db = db

	if c.cfg.AutoMigrate && len(c.models) > 0 {
		if err := c.db.AutoMigrate(c.models...); err != nil {
			return fmt.Errorf("database auto-migrate: %w", err)
		}
	}

	return nil
}

// Stop gracefully closes the database connection.
func (c *Component) Stop(_ context.Context) error {
	if c.db == nil {
		return nil
	}
	return c.db.Close()
}

// Health returns the current health status of the database.
func (c *Component) Health(ctx context.Context) component.Health {
	if c.db == nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "database not initialized",
		}
	}

	if err := c.db.Ping(); err != nil {
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

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	details := fmt.Sprintf("DSN: %s, MaxConns: %d", maskDSN(c.cfg.DSN), c.cfg.MaxOpenConns)
	if c.cfg.AutoMigrate {
		details += ", auto-migrate=on"
	}
	return component.Description{
		Name:    "Database",
		Type:    "database",
		Details: details,
	}
}

// maskDSN hides sensitive parts of the DSN for display
func maskDSN(dsn string) string {
	if len(dsn) <= 20 {
		return "***"
	}
	return dsn[:10] + "***"
}
