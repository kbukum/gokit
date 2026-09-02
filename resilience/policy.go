package resilience

import (
	"context"
	"sync"
	"time"
)

// TimeoutMode controls how a policy applies its timeout budget.
type TimeoutMode int

const (
	// TimeoutOverrideExisting applies the timeout budget even when the incoming context already has a deadline,
	// effectively choosing the earlier deadline.
	TimeoutOverrideExisting TimeoutMode = iota
	// TimeoutIfUnset applies the timeout budget only when the incoming context does not already carry a deadline.
	TimeoutIfUnset
)

// Policy composes resilience primitives into a single reusable execution policy.
//
// Its exported fields carry snake_case json/mapstructure/yaml tags so a Policy can be
// loaded from configuration and round-tripped through JSON (e.g. an httpclient
// resilience_policy block). Callback and RNG fields are not serializable and are
// tagged json:"-" so encoding a configured policy never fails on a function value.
type Policy struct {
	Retry          *RetryConfig          `json:"retry,omitempty" yaml:"retry" mapstructure:"retry"`
	CircuitBreaker *CircuitBreakerConfig `json:"circuit_breaker,omitempty" yaml:"circuit_breaker" mapstructure:"circuit_breaker"`
	Bulkhead       *BulkheadConfig       `json:"bulkhead,omitempty" yaml:"bulkhead" mapstructure:"bulkhead"`
	RateLimiter    *RateLimiterConfig    `json:"rate_limiter,omitempty" yaml:"rate_limiter" mapstructure:"rate_limiter"`
	Timeout        time.Duration         `json:"timeout,omitempty" yaml:"timeout" mapstructure:"timeout"`
	timeoutMode    TimeoutMode

	once sync.Once
	cb   *CircuitBreaker
	bh   *Bulkhead
	rl   *RateLimiter
}

// NewPolicy creates an empty policy that can be configured fluently.
func NewPolicy() *Policy {
	return &Policy{}
}

// WithRetry configures retry behavior.
func (p *Policy) WithRetry(cfg RetryConfig) *Policy {
	p.Retry = &cfg
	return p
}

// WithCircuitBreaker configures circuit breaker behavior.
func (p *Policy) WithCircuitBreaker(cfg CircuitBreakerConfig) *Policy {
	p.CircuitBreaker = &cfg
	return p
}

// WithBulkhead configures bulkhead behavior.
func (p *Policy) WithBulkhead(cfg BulkheadConfig) *Policy {
	p.Bulkhead = &cfg
	return p
}

// WithRateLimiter configures rate limiting behavior.
func (p *Policy) WithRateLimiter(cfg RateLimiterConfig) *Policy {
	p.RateLimiter = &cfg
	return p
}

// WithTimeout configures the shared timeout budget for a single execution.
func (p *Policy) WithTimeout(d time.Duration) *Policy {
	p.Timeout = d
	p.timeoutMode = TimeoutOverrideExisting
	return p
}

// WithTimeoutIfUnset configures a timeout budget that is only applied when the incoming context does not already carry a deadline.
func (p *Policy) WithTimeoutIfUnset(d time.Duration) *Policy {
	p.Timeout = d
	p.timeoutMode = TimeoutIfUnset
	return p
}

func (p *Policy) init() {
	if p == nil {
		return
	}
	p.once.Do(func() {
		if p.CircuitBreaker != nil {
			p.cb = NewCircuitBreaker(*p.CircuitBreaker)
		}
		if p.Bulkhead != nil {
			p.bh = NewBulkhead(*p.Bulkhead)
		}
		if p.RateLimiter != nil {
			p.rl = NewRateLimiter(*p.RateLimiter)
		}
	})
}

// IsAvailable reports whether the policy's circuit breaker currently permits
// calls. It returns true when no breaker is configured or the breaker is not
// open, so a provider can delegate its own availability check to the policy
// instead of duplicating breaker state.
func (p *Policy) IsAvailable() bool {
	if p == nil {
		return true
	}
	p.init()
	return p.cb == nil || p.cb.State() != StateOpen
}

// Execute runs fn through the configured resilience stack.
//
// Execution order from outermost to innermost:
// rate limiter → bulkhead → circuit breaker → timeout → retry → fn.
func Execute[T any](ctx context.Context, p *Policy, fn func(ctx context.Context) (T, error)) (T, error) {
	if p == nil {
		return fn(ctx)
	}

	p.init()

	if p.rl != nil {
		if err := p.rl.Wait(ctx); err != nil {
			var zero T
			return zero, err
		}
	}

	call := fn
	if p.Retry != nil {
		retryCfg := *p.Retry
		inner := call
		call = func(callCtx context.Context) (T, error) {
			return Retry(callCtx, retryCfg, func() (T, error) {
				return inner(callCtx)
			})
		}
	}
	if p.Timeout > 0 {
		inner := call
		call = func(callCtx context.Context) (T, error) {
			if p.timeoutMode == TimeoutIfUnset {
				if _, ok := callCtx.Deadline(); ok {
					return inner(callCtx)
				}
			}
			timeoutCtx, cancel := context.WithTimeout(callCtx, p.Timeout)
			defer cancel()
			return inner(timeoutCtx)
		}
	}
	if p.cb != nil {
		inner := call
		call = func(callCtx context.Context) (T, error) {
			var result T
			var resultErr error
			cbErr := p.cb.Execute(func() error {
				result, resultErr = inner(callCtx)
				return resultErr
			})
			if cbErr != nil && resultErr == nil {
				return result, cbErr
			}
			return result, resultErr
		}
	}
	if p.bh != nil {
		inner := call
		return ExecuteWithResult(ctx, p.bh, func() (T, error) {
			return inner(ctx)
		})
	}

	return call(ctx)
}
