package redis

import (
	"fmt"
	"time"
)

// Config holds Redis connection configuration.
type Config struct {
	// Name identifies this adapter instance (used by provider.Provider interface).
	Name string `yaml:"name" mapstructure:"name"`

	// Enabled controls whether the Redis component is active.
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`

	// Addr is the Redis server address (host:port).
	Addr string `yaml:"addr" mapstructure:"addr"`

	// Password is the Redis server password.
	Password string `yaml:"password" mapstructure:"password"`

	// DB is the Redis database number.
	DB int `yaml:"db" mapstructure:"db"`

	// PoolSize is the maximum number of socket connections.
	PoolSize int `yaml:"pool_size" mapstructure:"pool_size"`

	// MinIdleConns is the minimum number of idle connections.
	MinIdleConns int `yaml:"min_idle_conns" mapstructure:"min_idle_conns"`

	// MaxRetries is the maximum number of retries before giving up (0 = default 3).
	MaxRetries int `yaml:"max_retries" mapstructure:"max_retries"`

	// MinRetryBackoff is the minimum backoff between retries (e.g. "8ms").
	MinRetryBackoff string `yaml:"min_retry_backoff" mapstructure:"min_retry_backoff"`

	// MaxRetryBackoff is the maximum backoff between retries (e.g. "512ms").
	MaxRetryBackoff string `yaml:"max_retry_backoff" mapstructure:"max_retry_backoff"`

	// DialTimeout is the timeout for establishing new connections (e.g. "5s").
	DialTimeout string `yaml:"dial_timeout" mapstructure:"dial_timeout"`

	// ReadTimeout is the timeout for socket reads (e.g. "3s").
	ReadTimeout string `yaml:"read_timeout" mapstructure:"read_timeout"`

	// WriteTimeout is the timeout for socket writes (e.g. "3s").
	WriteTimeout string `yaml:"write_timeout" mapstructure:"write_timeout"`

	// ConnMaxIdleTime is the maximum time a connection may sit idle before being closed (e.g. "5m").
	ConnMaxIdleTime string `yaml:"idle_timeout" mapstructure:"idle_timeout"`

	// PoolTimeout is the amount of time the client waits for a connection from the pool (e.g. "4s").
	PoolTimeout string `yaml:"pool_timeout" mapstructure:"pool_timeout"`

	// ConnMaxLifetime is the maximum time a connection may be reused (e.g. "30m"). 0 means no limit.
	ConnMaxLifetime string `yaml:"max_conn_age" mapstructure:"max_conn_age"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.PoolSize <= 0 {
		c.PoolSize = 10
	}
	if c.MinIdleConns <= 0 {
		c.MinIdleConns = 2
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
	if c.MinRetryBackoff == "" {
		c.MinRetryBackoff = "8ms"
	}
	if c.MaxRetryBackoff == "" {
		c.MaxRetryBackoff = "512ms"
	}
	if c.DialTimeout == "" {
		c.DialTimeout = "5s"
	}
	if c.ReadTimeout == "" {
		c.ReadTimeout = "3s"
	}
	if c.WriteTimeout == "" {
		c.WriteTimeout = "3s"
	}
}

// Validate checks that required fields are present and parseable.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // skip validation when disabled
	}
	if c.Addr == "" {
		return fmt.Errorf("redis addr is required")
	}
	if c.PoolSize <= 0 {
		return fmt.Errorf("pool_size must be > 0")
	}
	if _, err := time.ParseDuration(c.DialTimeout); err != nil {
		return fmt.Errorf("invalid dial_timeout %q: %w", c.DialTimeout, err)
	}
	if _, err := time.ParseDuration(c.ReadTimeout); err != nil {
		return fmt.Errorf("invalid read_timeout %q: %w", c.ReadTimeout, err)
	}
	if _, err := time.ParseDuration(c.WriteTimeout); err != nil {
		return fmt.Errorf("invalid write_timeout %q: %w", c.WriteTimeout, err)
	}
	return nil
}
