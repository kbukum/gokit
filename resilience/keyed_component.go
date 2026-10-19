package resilience

import (
	"context"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
	apperr "github.com/kbukum/gokit/errors"
)

// KeyedComponent owns keyed admission and its cleanup worker for an application lifecycle.
type KeyedComponent struct {
	name    string
	cfg     KeyedRateLimiterConfig
	mu      sync.RWMutex
	limiter *KeyedRateLimiter
}

var _ component.Component = (*KeyedComponent)(nil)

// NewKeyedComponent returns a stopped limiter. Start creates the bounded key set; Stop closes its worker. Outside that lifecycle, Allow denies admission. Zero config fields take the [KeyedRateLimiterConfig] defaults; negative ones are refused.
func NewKeyedComponent(name string, cfg KeyedRateLimiterConfig) (*KeyedComponent, error) {
	switch {
	case name == "":
		return nil, apperr.InvalidInput("name", "is required")
	case cfg.CleanupInterval < 0:
		return nil, apperr.InvalidInput("cleanup_interval", "must not be negative")
	case cfg.BucketTTL < 0:
		return nil, apperr.InvalidInput("bucket_ttl", "must not be negative")
	case cfg.MaxKeys < 0:
		return nil, apperr.InvalidInput("max_keys", "must not be negative")
	}
	return &KeyedComponent{name: name, cfg: cfg}, nil
}

func (c *KeyedComponent) Name() string { return c.name }

func (c *KeyedComponent) Start(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limiter == nil {
		c.limiter = NewKeyedRateLimiter(c.cfg)
	}
	return nil
}

func (c *KeyedComponent) Stop(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limiter != nil {
		c.limiter.Stop()
		c.limiter = nil
	}
	return nil
}

func (c *KeyedComponent) Health(context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.limiter == nil {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "limiter is not running"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

func (c *KeyedComponent) Allow(key string, limit int, interval time.Duration) RateLimitDecision {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.limiter == nil {
		return RateLimitDecision{Limit: limit, RetryAfter: time.Second, Saturated: true}
	}
	return c.limiter.Allow(key, limit, interval)
}
