package redis

import (
	"fmt"
	"time"
)

// Config holds Redis connection configuration.
type Config struct {
	// Enabled controls whether the Redis component is active.
	Enabled bool `mapstructure:"enabled"`

	// Addr is the Redis server address (host:port).
	Addr string `mapstructure:"addr"`

	// Password is the Redis server password.
	Password string `mapstructure:"password"`

	// DB is the Redis database number.
	DB int `mapstructure:"db"`

	// PoolSize is the maximum number of socket connections.
	PoolSize int `mapstructure:"pool_size"`

	// MinIdleConns is the minimum number of idle connections.
	MinIdleConns int `mapstructure:"min_idle_conns"`

	// DialTimeout is the timeout for establishing new connections (e.g. "5s").
	DialTimeout string `mapstructure:"dial_timeout"`

	// ReadTimeout is the timeout for socket reads (e.g. "3s").
	ReadTimeout string `mapstructure:"read_timeout"`

	// WriteTimeout is the timeout for socket writes (e.g. "3s").
	WriteTimeout string `mapstructure:"write_timeout"`
}

// ApplyDefaults sets sensible defaults for zero-valued fields.
func (c *Config) ApplyDefaults() {
	if c.Addr == "" {
		c.Addr = "localhost:6379"
	}
	if c.PoolSize <= 0 {
		c.PoolSize = 10
	}
	if c.MinIdleConns <= 0 {
		c.MinIdleConns = 2
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
