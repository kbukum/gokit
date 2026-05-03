package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kbukum/gokit/cache"
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

	log.Debug("Redis client created", map[string]interface{}{
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
func (c *Client) Get(ctx context.Context, key string) (value []byte, found bool, err error) {
	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return raw, true, nil
}

// Set stores a value with a key and expiration.
func (c *Client) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiration).Err()
}

// Delete deletes key.
func (c *Client) Delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}

// Exists checks if key exists.
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.rdb.Exists(ctx, key).Result()
	return n > 0, err
}

// GetJSON retrieves and unmarshals a JSON value from Redis.
// Returns an error if the key doesn't exist or the value can't be unmarshalled.
func (c *Client) GetJSON(ctx context.Context, key string, dest any) error {
	raw, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("redis get json %q: %w", key, err)
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("redis unmarshal %q: %w", key, err)
	}
	return nil
}

// SetJSON marshals a value to JSON and stores it in Redis with optional TTL.
func (c *Client) SetJSON(ctx context.Context, key string, val any, ttl time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("redis marshal %q: %w", key, err)
	}
	return c.rdb.Set(ctx, key, data, ttl).Err()
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
	c.log.Debug("Closing Redis connection")
	c.closed = true
	return c.rdb.Close()
}

// Unwrap returns the underlying go-redis client for advanced operations.
func (c *Client) Unwrap() *goredis.Client {
	return c.rdb
}

var (
	_ cache.Store      = (*Client)(nil)
	_ cache.CloseStore = (*Client)(nil)
)
