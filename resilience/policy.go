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
	err  error
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

// Clone returns a copy of the policy carrying only its configuration, with fresh
// lazily-initialized primitive state. The Retry block is deep-copied so a caller
// may safely default its non-serializable callbacks (for example RetryIf) on the
// clone without mutating — or racing — the source policy shared by other callers.
// Clone on a nil policy returns nil.
func (p *Policy) Clone() *Policy {
	if p == nil {
		return nil
	}
	c := &Policy{
		CircuitBreaker: p.CircuitBreaker,
		Bulkhead:       p.Bulkhead,
		RateLimiter:    p.RateLimiter,
		Timeout:        p.Timeout,
		timeoutMode:    p.timeoutMode,
	}
	if p.Retry != nil {
		retry := *p.Retry
		c.Retry = &retry
	}
	return c
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
			p.bh, p.err = NewBulkhead(*p.Bulkhead)
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
	return p.err == nil && (p.cb == nil || p.cb.State() != StateOpen)
}

// Execute runs fn through the configured resilience stack.
//
// Execution order from outermost to innermost:
// timeout → rate limiter → bulkhead → circuit breaker → retry → fn.
func Execute[T any](ctx context.Context, p *Policy, fn func(ctx context.Context) (T, error)) (T, error) {
	var retry *RetryConfig
	if p != nil {
		retry = p.Retry
	}
	return ExecuteWithRetry(ctx, p, retry, fn)
}

// ExecuteWithRetry selects the retry budget for one call without copying or resetting shared circuit-breaker, bulkhead or limiter state. A nil retry disables retries for that call.
func ExecuteWithRetry[T any](ctx context.Context, p *Policy, retry *RetryConfig, fn func(ctx context.Context) (T, error)) (result T, err error) {
	callCtx, finish, err := p.Acquire(ctx)
	if err != nil {
		return result, err
	}
	defer func() { err = finish(err) }()
	// A callback that panics must release admission without reporting success.
	err = context.Canceled

	if retry == nil {
		return fn(callCtx)
	}
	return Retry(callCtx, *retry, func() (T, error) {
		return fn(callCtx)
	})
}
