package resilience

import (
	"math"

	apperr "github.com/kbukum/gokit/errors"
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

// Validate rejects an inconsistent bulkhead configuration.
func (c BulkheadConfig) Validate() error {
	if c.MaxConcurrent < 1 {
		return apperr.InvalidInput("max_concurrent", "must be at least 1")
	}
	if c.MaxWait < 0 {
		return apperr.InvalidInput("max_wait", "must not be negative")
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
