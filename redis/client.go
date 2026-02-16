// Package redis provides a Redis client wrapper built on go-redis
// with gokit logging, connection pooling, and component lifecycle support.
package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kbukum/gokit/logger"
)

// Client wraps a go-redis client with gokit logging.
type Client struct {
	rdb    *goredis.Client
	log    *logger.Logger
	cfg    Config
	closed bool
	mu     sync.Mutex
}

// New creates a new Redis client with the given configuration and logger.
func New(cfg Config, log *logger.Logger) (*Client, error) {
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("redis config: %w", err)
	}

	if !cfg.Enabled {
		return nil, fmt.Errorf("redis is disabled")
	}

	dialTimeout, _ := time.ParseDuration(cfg.DialTimeout)
	readTimeout, _ := time.ParseDuration(cfg.ReadTimeout)
	writeTimeout, _ := time.ParseDuration(cfg.WriteTimeout)

	opts := &goredis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxRetries:   cfg.MaxRetries,
		DialTimeout:  dialTimeout,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}

	if cfg.MinRetryBackoff != "" {
		if d, err := time.ParseDuration(cfg.MinRetryBackoff); err == nil {
			opts.MinRetryBackoff = d
		}
	}
	if cfg.MaxRetryBackoff != "" {
		if d, err := time.ParseDuration(cfg.MaxRetryBackoff); err == nil {
			opts.MaxRetryBackoff = d
		}
	}
	if cfg.ConnMaxIdleTime != "" {
		if d, err := time.ParseDuration(cfg.ConnMaxIdleTime); err == nil {
			opts.ConnMaxIdleTime = d
		}
	}
	if cfg.PoolTimeout != "" {
		if d, err := time.ParseDuration(cfg.PoolTimeout); err == nil {
			opts.PoolTimeout = d
		}
	}
	if cfg.ConnMaxLifetime != "" {
		if d, err := time.ParseDuration(cfg.ConnMaxLifetime); err == nil {
			opts.ConnMaxLifetime = d
		}
	}

	rdb := goredis.NewClient(opts)

	log.Info("Redis client created", map[string]interface{}{
		"addr":      cfg.Addr,
		"db":        cfg.DB,
		"pool_size": cfg.PoolSize,
	})

	return &Client{rdb: rdb, log: log, cfg: cfg}, nil
}

// Ping verifies the Redis connection is alive.
func (c *Client) Ping(ctx context.Context) error {
	pong, err := c.rdb.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	if pong != "PONG" {
		return fmt.Errorf("unexpected redis ping response: %s", pong)
	}
	return nil
}

// Get retrieves a value by key.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

// Set stores a value with a key and expiration.
func (c *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiration).Err()
}

// Del deletes one or more keys.
func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

// Exists checks if one or more keys exist.
func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.rdb.Exists(ctx, keys...).Result()
}

// Close closes the Redis connection. Safe to call multiple times.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.log.Info("Closing Redis connection")
	c.closed = true
	return c.rdb.Close()
}

// Unwrap returns the underlying go-redis client for advanced operations.
func (c *Client) Unwrap() *goredis.Client {
	return c.rdb
}
