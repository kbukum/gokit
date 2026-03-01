package redis

import (
	"context"

	"github.com/kbukum/gokit/provider"
)

// compile-time assertion
var _ provider.Provider = (*Client)(nil)

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
