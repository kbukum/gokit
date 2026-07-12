package database

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/util"
)

// Component wraps DB and implements component.Component for lifecycle management.
type Component struct {
	db         *DB
	cfg        Config
	log        *logging.Logger
	models     []any
	driverFunc DriverFunc
	driverName string
}

// NewComponent creates a database component for use with the component registry.
// Drivers are opt-in: call WithDriver or WithDriverFromRegistry before Start.
// The Config.Enabled flag can be used to skip initialization at runtime.
func NewComponent(cfg Config, log *logging.Logger) *Component {
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
func (c *Component) WithDriver(fn DriverFunc) *Component {
	c.driverFunc = fn
	c.driverName = ""
	return c
}

// WithDriverFromRegistry selects a registered driver by name.
func (c *Component) WithDriverFromRegistry(reg *DriverRegistry, name string) *Component {
	c.driverName = name
	c.driverFunc = nil
	if reg == nil {
		return c
	}
	if fn, ok := reg.Get(name); ok {
		c.driverFunc = fn
	}
	return c
}

// WithAutoMigrate registers models for auto-migration on Start.
// Models are only migrated if Config.AutoMigrate is true and the component is enabled.
func (c *Component) WithAutoMigrate(models ...any) *Component {
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
// If Config.Enabled is false, this method returns immediately without error.
// The context is used for connection retries and can be canceled to abort startup.
func (c *Component) Start(ctx context.Context) error {
	if !c.cfg.Enabled {
		c.log.InfoCtx(ctx, "Database component is disabled")
		return nil
	}

	if c.driverFunc == nil {
		if c.driverName != "" {
			return fmt.Errorf("database start: driver %q is not configured; register the adapter and call WithDriverFromRegistry or call WithDriver directly", c.driverName)
		}
		return fmt.Errorf("database start: driver is not configured; register an adapter and call WithDriverFromRegistry or call WithDriver directly")
	}

	dialector := c.driverFunc(c.cfg.BuildDSN())

	// Create connection using the dialector with context support
	db, err := NewWithContext(ctx, dialector, c.cfg, c.log)
	if err != nil {
		return fmt.Errorf("database start: %w", err)
	}
	c.db = db

	if c.cfg.AutoMigrate && len(c.models) > 0 {
		if err := c.db.AutoMigrate(c.models...); err != nil { //nolint:contextcheck // AutoMigrate is a synchronous schema operation without a request context
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
	return c.db.Close() //nolint:contextcheck // Close is invoked from lifecycle Stop without a request context
}

// Health returns the current health status of the database.
// If Config.Enabled is false, returns StatusHealthy with "disabled" message.
// The context is used for the ping operation and honors cancellation.
func (c *Component) Health(ctx context.Context) component.Health {
	if !c.cfg.Enabled {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusHealthy,
			Message: "disabled",
		}
	}

	if c.db == nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "database not initialized",
		}
	}

	if err := c.db.PingContext(ctx); err != nil {
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
	details := fmt.Sprintf("DSN: %s, MaxConns: %d", util.MaskSecret(c.cfg.BuildDSN(), 10), c.cfg.MaxOpenConns)
	if c.cfg.AutoMigrate {
		details += ", auto-migrate=on"
	}
	return component.Description{
		Name:    "Database",
		Type:    "database",
		Details: details,
	}
}
