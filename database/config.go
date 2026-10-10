package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/kbukum/gokit/util"
)

// Config holds database connection configuration.
//
// Config is driver-agnostic: it carries an opaque [Config.DSN] connection string or structured
// [Config.Params], plus pool, retry, and logging settings. Interpreting Params is owned by
// the selected dialect, so the core module never
// needs to know any driver's connection-string shape.
type Config struct {
	// Name identifies this adapter instance (used by provider.Provider interface).
	Name string `yaml:"name" mapstructure:"name"`

	// Enabled controls whether the database component is active.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// DSN is the opaque database connection string supplied by the caller. Its format is
	// defined by the selected driver adapter; core treats it as an opaque value. When set, it
	// is mutually exclusive with Params.
	DSN string `yaml:"dsn" mapstructure:"dsn"`

	// Params are structured parameters interpreted by the backend, mutually exclusive with DSN.
	Params ConnParams `yaml:"params" mapstructure:"params"`

	// MaxOpenConns sets the maximum number of open connections to the database.
	MaxOpenConns int `yaml:"max_open_conns" mapstructure:"max_open_conns"`

	// MaxIdleConns sets the maximum number of idle connections in the pool.
	MaxIdleConns int `yaml:"max_idle_conns" mapstructure:"max_idle_conns"`

	// ConnMaxLifetime is the maximum time a connection may be reused (e.g. "1h", "30m").
	ConnMaxLifetime string `yaml:"conn_max_lifetime" mapstructure:"conn_max_lifetime"`

	// ConnMaxIdleTime is the maximum time a connection may sit idle (e.g. "5m", "10m"). If empty,
	// no idle timeout is set.
	ConnMaxIdleTime string `yaml:"conn_max_idle_time" mapstructure:"conn_max_idle_time"`

	// MaxRetries is the number of connection attempts before giving up.
	MaxRetries int `yaml:"max_retries" mapstructure:"max_retries"`

	// ConnectTimeout bounds each individual connection attempt (e.g. "30s"). It guards against a
	// blackholed endpoint blocking an attempt for the driver's unbounded default. Empty defaults
	// to "30s"; set "0" to disable the per-attempt bound and rely solely on the caller's context.
	ConnectTimeout string `yaml:"connect_timeout" mapstructure:"connect_timeout"`

	// AutoMigrate controls whether GORM auto-migration runs on startup.
	AutoMigrate bool `yaml:"auto_migrate" mapstructure:"auto_migrate"`

	// SlowQueryThreshold is the duration above which queries are logged as slow (e.g. "200ms").
	SlowQueryThreshold string `yaml:"slow_query_threshold" mapstructure:"slow_query_threshold"`

	// LogLevel controls GORM's log verbosity: "silent", "error", "warn", "info" (default).
	LogLevel string `yaml:"log_level" mapstructure:"log_level"`
}

func (Config) String() string     { return "Database configuration (credentials redacted)" }
func (c Config) GoString() string { return c.String() }

func (c Config) MarshalJSON() ([]byte, error) {
	type fields Config
	return json.Marshal(struct {
		fields
		DSN util.SecretString
	}{fields: fields(c), DSN: util.NewSecretString(c.DSN)})
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.MaxOpenConns == 0 {
		c.MaxOpenConns = 25
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 5
	}
	if c.ConnMaxLifetime == "" {
		c.ConnMaxLifetime = "1h"
	}
	if c.ConnMaxIdleTime == "" {
		c.ConnMaxIdleTime = "5m"
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = 5
	}
	if c.ConnectTimeout == "" {
		c.ConnectTimeout = "30s"
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
	return c.validateConnection()
}

func (c Config) validateConnection() error {
	if c.DSN == "" && c.Params.IsZero() {
		return fmt.Errorf("database: dsn or params is required")
	}
	if err := (ConnectionInput{DSN: c.DSN, Params: c.Params}).Validate(); err != nil {
		return err
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
	if c.ConnectTimeout != "" {
		timeout, err := time.ParseDuration(c.ConnectTimeout)
		if err != nil {
			return fmt.Errorf("invalid connect_timeout %q: %w", c.ConnectTimeout, err)
		}
		if timeout < 0 {
			return fmt.Errorf("connect_timeout must be >= 0")
		}
	}
	return nil
}
