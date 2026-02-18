package database

import (
	"fmt"
	"time"
)

// Config holds database connection configuration.
type Config struct {
	// Enabled controls whether the database component is active.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// DSN is the database connection string.
	DSN string `yaml:"dsn" mapstructure:"dsn"`

	// MaxOpenConns sets the maximum number of open connections to the database.
	MaxOpenConns int `yaml:"max_open_conns" mapstructure:"max_open_conns"`

	// MaxIdleConns sets the maximum number of idle connections in the pool.
	MaxIdleConns int `yaml:"max_idle_conns" mapstructure:"max_idle_conns"`

	// ConnMaxLifetime is the maximum time a connection may be reused (e.g. "1h", "30m").
	ConnMaxLifetime string `yaml:"conn_max_lifetime" mapstructure:"conn_max_lifetime"`

	// ConnMaxIdleTime is the maximum time a connection may sit idle (e.g. "5m", "10m").
	// If empty, no idle timeout is set.
	ConnMaxIdleTime string `yaml:"conn_max_idle_time" mapstructure:"conn_max_idle_time"`

	// MaxRetries is the number of connection attempts before giving up.
	MaxRetries int `yaml:"max_retries" mapstructure:"max_retries"`

	// AutoMigrate controls whether GORM auto-migration runs on startup.
	AutoMigrate bool `yaml:"auto_migrate" mapstructure:"auto_migrate"`

	// SlowQueryThreshold is the duration above which queries are logged as slow (e.g. "200ms").
	SlowQueryThreshold string `yaml:"slow_query_threshold" mapstructure:"slow_query_threshold"`

	// LogLevel controls GORM's log verbosity: "silent", "error", "warn", "info" (default).
	LogLevel string `yaml:"log_level" mapstructure:"log_level"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 25
	}
	if c.MaxIdleConns <= 0 {
		c.MaxIdleConns = 5
	}
	if c.ConnMaxLifetime == "" {
		c.ConnMaxLifetime = "1h"
	}
	if c.ConnMaxIdleTime == "" {
		c.ConnMaxIdleTime = "5m"
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 5
	}
	if c.SlowQueryThreshold == "" {
		c.SlowQueryThreshold = "200ms"
	}
	if c.LogLevel == "" {
		c.LogLevel = "warn"
	}
}

// Validate checks that required fields are present and parseable.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // skip validation when disabled
	}
	if c.DSN == "" {
		return fmt.Errorf("database DSN is required")
	}
	
	if c.MaxOpenConns <= 0 {
		return fmt.Errorf("max_open_conns must be > 0")
	}
	if c.MaxIdleConns <= 0 {
		return fmt.Errorf("max_idle_conns must be > 0")
	}
	if c.MaxIdleConns > c.MaxOpenConns {
		return fmt.Errorf("max_idle_conns (%d) must be <= max_open_conns (%d)", c.MaxIdleConns, c.MaxOpenConns)
	}
	if _, err := time.ParseDuration(c.ConnMaxLifetime); err != nil {
		return fmt.Errorf("invalid conn_max_lifetime %q: %w", c.ConnMaxLifetime, err)
	}
	if c.ConnMaxIdleTime != "" {
		if _, err := time.ParseDuration(c.ConnMaxIdleTime); err != nil {
			return fmt.Errorf("invalid conn_max_idle_time %q: %w", c.ConnMaxIdleTime, err)
		}
	}
	if _, err := time.ParseDuration(c.SlowQueryThreshold); err != nil {
		return fmt.Errorf("invalid slow_query_threshold %q: %w", c.SlowQueryThreshold, err)
	}
	if c.MaxRetries <= 0 {
		return fmt.Errorf("max_retries must be > 0")
	}
	return nil
}
