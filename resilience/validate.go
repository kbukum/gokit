package resilience

import (
	"math"

	apperr "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Validate rejects an inconsistent circuit-breaker configuration.
func (c CircuitBreakerConfig) Validate() error {
	if c.MaxFailures < 1 {
		return apperr.InvalidInput("max_failures", "must be at least 1")
	}
	if c.Timeout < 0 {
		return apperr.InvalidInput("timeout", "must not be negative")
	}
	if c.HalfOpenMaxCalls < 1 {
		return apperr.InvalidInput("half_open_max_calls", "must be at least 1")
	}
	return nil
}

// Validate rejects an inconsistent bulkhead configuration. It is the single rule set NewBulkhead enforces: zero MaxQueue means no waiting, and a positive MaxQueue requires a positive MaxWait so every waiter is bounded in time and count.
func (c BulkheadConfig) Validate() error {
	if c.MaxConcurrent < 1 {
		return apperr.InvalidInput("max_concurrent", "must be at least 1")
	}
	if c.MaxWait < 0 {
		return apperr.InvalidInput("max_wait", "must not be negative")
	}
	if c.MaxQueue < 0 {
		return apperr.InvalidInput("max_queue", "must not be negative")
	}
	if c.MaxQueue > 0 && c.MaxWait == 0 {
		return apperr.InvalidInput("max_wait", "a positive max_queue requires a positive max_wait")
	}
	if c.Clock != nil && util.IsNil(c.Clock) {
		return apperr.InvalidInput("clock", "cannot be typed nil")
	}
	return nil
}

// Validate rejects a policy whose primitives cannot be constructed. Circuit-breaker, rate-limiter and retry zero fields are defaulted by their constructors; explicit invalid values and every bulkhead rule are rejected here, before the policy is published.
func (p *Policy) Validate() error {
	if p == nil {
		return nil
	}
	if p.Timeout < 0 {
		return apperr.InvalidInput("timeout", "must not be negative")
	}
	if p.Bulkhead != nil {
		if err := p.Bulkhead.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate rejects an inconsistent rate-limiter configuration.
func (c RateLimiterConfig) Validate() error {
	if c.Rate <= 0 || math.IsNaN(c.Rate) || math.IsInf(c.Rate, 0) {
		return apperr.InvalidInput("rate", "must be a finite number greater than 0")
	}
	if c.Burst < 1 {
		return apperr.InvalidInput("burst", "must be at least 1")
	}
	return nil
}
