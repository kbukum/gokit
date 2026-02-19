package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/testutil"
)

// Component is a test Redis component backed by miniredis (in-memory).
// It implements both component.Component and testutil.TestComponent.
type Component struct {
	mini    *miniredis.Miniredis
	client  *goredis.Client
	started bool
	mu      sync.RWMutex
}

var _ component.Component = (*Component)(nil)
var _ testutil.TestComponent = (*Component)(nil)

// NewComponent creates a new in-memory Redis test component.
func NewComponent() *Component {
	return &Component{}
}

// Client returns the underlying *goredis.Client, or nil if not started.
func (c *Component) Client() *goredis.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

// Name returns the component name.
func (c *Component) Name() string { return "redis-test" }

// Start launches the in-memory Redis server.
func (c *Component) Start(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("component already started")
	}

	mini, err := miniredis.Run()
	if err != nil {
		return fmt.Errorf("failed to start miniredis: %w", err)
	}

	c.mini = mini
	c.client = goredis.NewClient(&goredis.Options{Addr: mini.Addr()})
	c.started = true
	return nil
}

// Stop shuts down the in-memory Redis server.
func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.started {
		return nil
	}

	if c.client != nil {
		_ = c.client.Close()
	}
	if c.mini != nil {
		c.mini.Close()
	}
	c.started = false
	return nil
}

// Health returns the health status.
func (c *Component) Health(_ context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "not started",
		}
	}
	return component.Health{
		Name:   c.Name(),
		Status: component.StatusHealthy,
	}
}

// Reset flushes all keys from the in-memory Redis.
func (c *Component) Reset(_ context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.mini == nil {
		return fmt.Errorf("component not started")
	}
	c.mini.FlushAll()
	return nil
}

// Snapshot captures the current Redis state.
// Returns a map of key→value for all string keys.
func (c *Component) Snapshot(_ context.Context) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.mini == nil {
		return nil, fmt.Errorf("component not started")
	}

	snapshot := make(map[string]string)
	for _, key := range c.mini.Keys() {
		val, err := c.mini.Get(key)
		if err == nil {
			snapshot[key] = val
		}
	}
	return snapshot, nil
}

// Restore returns the Redis state to a previously captured snapshot.
func (c *Component) Restore(_ context.Context, snap interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.started || c.mini == nil {
		return fmt.Errorf("component not started")
	}

	snapshot, ok := snap.(map[string]string)
	if !ok {
		return fmt.Errorf("invalid snapshot type: expected map[string]string, got %T", snap)
	}

	c.mini.FlushAll()
	for key, val := range snapshot {
		if err := c.mini.Set(key, val); err != nil {
			return fmt.Errorf("failed to restore key %q: %w", key, err)
		}
	}
	return nil
}
