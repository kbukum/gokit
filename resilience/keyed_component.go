package resilience

import (
	"context"
	"sync"
	"time"

	"github.com/kbukum/gokit/component"
)

// KeyedComponent owns keyed admission and its cleanup worker for an application lifecycle.
type KeyedComponent interface {
	component.Component
	Allow(key string, limit int, interval time.Duration) RateLimitDecision
}

type keyedComponent struct {
	name    string
	cfg     KeyedRateLimiterConfig
	mu      sync.RWMutex
	limiter *KeyedRateLimiter
}

// NewKeyedComponent returns a stopped limiter. Start creates the bounded key set; Stop closes its worker. Outside that lifecycle, Allow denies admission.
func NewKeyedComponent(name string, cfg KeyedRateLimiterConfig) KeyedComponent {
	return &keyedComponent{name: name, cfg: cfg}
}

func (c *keyedComponent) Name() string { return c.name }

func (c *keyedComponent) Start(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limiter == nil {
		c.limiter = NewKeyedRateLimiter(c.cfg)
	}
	return nil
}

func (c *keyedComponent) Stop(context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.limiter != nil {
		c.limiter.Stop()
		c.limiter = nil
	}
	return nil
}

func (c *keyedComponent) Health(context.Context) component.Health {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.limiter == nil {
		return component.Health{Name: c.Name(), Status: component.StatusUnhealthy, Message: "limiter is not running"}
	}
	return component.Health{Name: c.Name(), Status: component.StatusHealthy}
}

func (c *keyedComponent) Allow(key string, limit int, interval time.Duration) RateLimitDecision {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.limiter == nil {
		return RateLimitDecision{Limit: limit, RetryAfter: time.Second, Saturated: true}
	}
	return c.limiter.Allow(key, limit, interval)
}
