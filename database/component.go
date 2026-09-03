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
	db          *DB
	cfg         Config
	log         *logging.Logger
	models      []any
	dialect     Dialect
	dialectName string
}

// NewComponent creates a database component for use with the component registry.
// Dialects are opt-in: call WithDialect or WithDialectFromRegistry before Start.
// The Config.Enabled flag can be used to skip initialization at runtime.
func NewComponent(cfg Config, log *logging.Logger) *Component {
	return &Component{
		cfg: cfg,
		log: log.WithComponent("database"),
	}
}

// WithDialect sets the database backend dialect directly.
//
// Example:
//
//	import "github.com/kbukum/gokit/database/postgres"
//
//	db := database.NewComponent(cfg, log).
//		WithDialect(postgres.Dialect()).
//		WithAutoMigrate(&User{}, &Post{})
func (c *Component) WithDialect(d Dialect) *Component {
	c.dialect = d
	c.dialectName = ""
	return c
}

// WithDialectFromRegistry selects a registered dialect by backend name.
func (c *Component) WithDialectFromRegistry(reg *DialectRegistry, name string) *Component {
	c.dialectName = name
	c.dialect = nil
	if reg == nil {
		return c
	}
	if d, ok := reg.Get(name); ok {
		c.dialect = d
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

// Start connects to the database and optionally runs auto-migration. If Config.Enabled is false,
// this method returns immediately without error. The context is used for connection retries
// and can be canceled to abort startup.
func (c *Component) Start(ctx context.Context) error {
	if !c.cfg.Enabled {
		c.log.InfoCtx(ctx, "Database component is disabled")
		return nil
	}

	// Apply defaults and validate before doing any work so a misconfigured component (e.g. neither
	// DSN nor Params set, or an unparseable duration) fails fast instead of attempting to connect
	// with a zero-value DSN built from empty structured params.
	c.cfg.ApplyDefaults()
	if err := c.cfg.Validate(); err != nil {
		return fmt.Errorf("database start: %w", err)
	}

	if c.dialect == nil {
		if c.dialectName != "" {
			return fmt.Errorf("database start: dialect %q is not configured; register the adapter and call WithDialectFromRegistry or call WithDialect directly", c.dialectName)
		}
		return fmt.Errorf("database start: dialect is not configured; register an adapter and call WithDialectFromRegistry or call WithDialect directly")
	}

	dsn, err := c.resolveDSN()
	if err != nil {
		return fmt.Errorf("database start: %w", err)
	}
	dialector := c.dialect.Open(dsn)

	// Create connection using the dialector with context support
	db, err := NewWithContext(ctx, dialector, c.cfg, c.log)
	if err != nil {
		return fmt.Errorf("database start: %w", err)
	}
	c.db = db

	if c.cfg.AutoMigrate && len(c.models) > 0 {
		if err := c.db.AutoMigrate(c.models...); err != nil { //nolint:contextcheck // AutoMigrate is a synchronous schema operation without a request context
			// The registry only calls Stop for components whose Start fails with a context
			// error, so release the pool we just opened before returning any other failure.
			if closeErr := c.db.Close(); closeErr != nil { //nolint:contextcheck // cleanup on the failure path, no request context
				c.log.ErrorCtx(ctx, "Failed to close database after auto-migrate failure", map[string]any{
					"error": closeErr.Error(),
				})
			}
			c.db = nil
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

// Health returns the current health status of the database. If Config.Enabled is false,
// returns StatusHealthy with "disabled" message. The context is used for the ping operation
// and honors cancellation.
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

// resolveDSN returns the effective DSN: the verbatim Config.DSN when set, otherwise the DSN built
// by the selected dialect from Config.Params. It errors when neither a DSN nor a structured-capable
// dialect is available.
func (c *Component) resolveDSN() (string, error) {
	if c.cfg.DSN != "" {
		return c.cfg.DSN, nil
	}
	sd, ok := c.dialect.(StructuredDialect)
	if !ok {
		return "", fmt.Errorf("dialect %q cannot build a DSN from structured params; set Config.DSN directly", c.selectedName())
	}
	return sd.DSN(c.cfg.Params)
}

// selectedName reports the configured dialect name, whether set directly or by registry lookup.
func (c *Component) selectedName() string {
	if c.dialect != nil {
		return c.dialect.Name()
	}
	return c.dialectName
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	target, _ := c.resolveDSN()
	details := fmt.Sprintf("DSN: %s, MaxConns: %d", util.MaskSecret(target, 10), c.cfg.MaxOpenConns)
	if c.cfg.AutoMigrate {
		details += ", auto-migrate=on"
	}
	return component.Description{
		Name:    "Database",
		Type:    "database",
		Details: details,
	}
}
