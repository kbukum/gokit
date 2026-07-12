package redis

import (
	"context"
	"fmt"

	"github.com/kbukum/gokit/cache"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/provider"
)

// compile-time assertion
var _ provider.Provider = (*Client)(nil)

// Register registers the Redis backend in an explicit cache registry.
func Register(reg *cache.FactoryRegistry) error {
	return reg.Register(cache.ProviderRedis, func(_ cache.Config, providerCfg any, log *logging.Logger) (cache.Store, error) {
		cfg, ok := providerCfg.(*Config)
		if !ok {
			return nil, &cache.ConfigTypeError{Provider: cache.ProviderRedis, Expected: "*redis.Config", Actual: providerCfg}
		}
		client, err := New(*cfg, log)
		if err != nil {
			return nil, fmt.Errorf("redis cache: %w", err)
		}
		return client, nil
	})
}

// Name returns the adapter name (implements provider.Provider).
func (c *Client) Name() string {
	return c.cfg.Name
}

// IsAvailable checks if the Redis client is connected (implements provider.Provider).
func (c *Client) IsAvailable(ctx context.Context) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	return c.rdb.Ping(ctx).Err() == nil
}
