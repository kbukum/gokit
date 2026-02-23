package provider

import (
	"github.com/kbukum/gokit/resilience"
)

// ResilienceConfig bundles optional resilience policies for a provider.
// Nil fields are skipped — zero config means pure passthrough with no overhead.
type ResilienceConfig struct {
	// CircuitBreaker prevents cascading failures by stopping calls after repeated errors.
	CircuitBreaker *resilience.CircuitBreakerConfig
	// Retry automatically retries failed calls with exponential backoff.
	Retry *resilience.RetryConfig
	// RateLimiter limits the rate of calls using a token bucket algorithm.
	RateLimiter *resilience.RateLimiterConfig
	// Bulkhead limits concurrent calls to prevent resource exhaustion.
	Bulkhead *resilience.BulkheadConfig
}

// IsEmpty returns true if no resilience policies are configured.
func (c ResilienceConfig) IsEmpty() bool {
	return c.CircuitBreaker == nil && c.Retry == nil && c.RateLimiter == nil && c.Bulkhead == nil
}

// ResilienceState holds initialized resilience primitives built from config.
type ResilienceState struct {
	cb *resilience.CircuitBreaker
	rl *resilience.RateLimiter
	bh *resilience.Bulkhead
	// Retry config is stored as-is since resilience.Retry is a function, not a struct.
	retryCfg *resilience.RetryConfig
}

// BuildResilience creates initialized resilience primitives from config.
func BuildResilience(cfg ResilienceConfig) *ResilienceState {
	if cfg.IsEmpty() {
		return nil
	}
	s := &ResilienceState{
		retryCfg: cfg.Retry,
	}
	if cfg.CircuitBreaker != nil {
		s.cb = resilience.NewCircuitBreaker(*cfg.CircuitBreaker)
	}
	if cfg.RateLimiter != nil {
		s.rl = resilience.NewRateLimiter(*cfg.RateLimiter)
	}
	if cfg.Bulkhead != nil {
		s.bh = resilience.NewBulkhead(*cfg.Bulkhead)
	}
	return s
}
