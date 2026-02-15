package database

import (
	"context"
	"fmt"
	"time"

	"github.com/skillsenselab/gokit/component"
	"github.com/skillsenselab/gokit/logger"
)

// Component wraps DB and implements component.Component for lifecycle management.
type Component struct {
	db     *DB
	cfg    Config
	log    *logger.Logger
	models []interface{}
}

// NewComponent creates a database component for use with the component registry.
func NewComponent(cfg Config, log *logger.Logger) *Component {
	return &Component{
		cfg: cfg,
		log: log.WithComponent("database"),
	}
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
	db, err := New(c.cfg, c.log)
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
func (c *Component) Health(ctx context.Context) component.ComponentHealth {
	if c.db == nil {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "database not initialized",
		}
	}

	if err := c.db.Ping(); err != nil {
		return component.ComponentHealth{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("ping failed: %v", err),
		}
	}

	return component.ComponentHealth{
		Name:   c.Name(),
		Status: component.StatusHealthy,
	}
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	details := fmt.Sprintf("pool=%d/%d", c.cfg.MaxOpenConns, c.cfg.MaxIdleConns)
	if c.cfg.AutoMigrate {
		details += " auto-migrate=on"
	}
	return component.Description{
		Name:    "Database",
		Type:    "database",
		Details: details,
	}
}
type HealthStatus struct {
	Connected  bool          `json:"connected"`
	Error      string        `json:"error,omitempty"`
	Latency    time.Duration `json:"latency"`
	OpenConns  int           `json:"open_connections"`
	InUseConns int           `json:"in_use_connections"`
	IdleConns  int           `json:"idle_connections"`
}

// IsHealthy returns true if the database connection is alive.
func (d *DB) IsHealthy(ctx context.Context) bool {
	health := d.CheckHealth(ctx)
	return health.Connected
}

// WaitForConnection waits for the database to become available, polling every
// second until the context deadline or timeout is reached.
func (d *DB) WaitForConnection(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if d.IsHealthy(ctx) {
				d.log.Info("Database connection established")
				return nil
			}
			d.log.Debug("Waiting for database connection...")
		}
	}
}

// CheckHealth performs a comprehensive database health check.
func (d *DB) CheckHealth(ctx context.Context) HealthStatus {
	start := time.Now()

	sqlDB, err := d.GormDB.DB()
	if err != nil {
		return HealthStatus{Connected: false, Error: err.Error(), Latency: time.Since(start)}
	}

	if err := sqlDB.PingContext(ctx); err != nil {
		return HealthStatus{Connected: false, Error: err.Error(), Latency: time.Since(start)}
	}

	stats := sqlDB.Stats()
	return HealthStatus{
		Connected:  true,
		Latency:    time.Since(start),
		OpenConns:  stats.OpenConnections,
		InUseConns: stats.InUse,
		IdleConns:  stats.Idle,
	}
}
