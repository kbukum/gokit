package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Common rate limiter errors.
var (
	ErrRateLimited = errors.New("rate limit exceeded")
)

// RateLimiterConfig configures a rate limiter.
type RateLimiterConfig struct {
	// Name identifies this rate limiter for metrics/logging.
	Name string
	// Rate is the number of requests allowed per second.
	Rate float64
	// Burst is the maximum burst size.
	Burst int
	// OnLimit is called when a request is rate limited.
	OnLimit func(name string)
}

// DefaultRateLimiterConfig returns sensible defaults.
func DefaultRateLimiterConfig(name string) RateLimiterConfig {
	return RateLimiterConfig{
		Name:  name,
		Rate:  10.0, // 10 requests per second
		Burst: 20,   // Allow bursts up to 20
	}
}

// RateLimiter implements a token bucket rate limiter.
// It controls the rate of requests to prevent overwhelming services.
type RateLimiter struct {
	config RateLimiterConfig

	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(config RateLimiterConfig) *RateLimiter {
	if config.Rate <= 0 {
		config.Rate = 10.0
	}
	if config.Burst <= 0 {
		config.Burst = int(config.Rate)
	}

	return &RateLimiter{
		config:     config,
		tokens:     float64(config.Burst),
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed without blocking.
// Returns true if allowed, false if rate limited.
func (rl *RateLimiter) Allow() bool {
	return rl.AllowN(1)
}

// AllowN checks if n requests are allowed without blocking.
func (rl *RateLimiter) AllowN(n int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	if rl.tokens >= float64(n) {
		rl.tokens -= float64(n)
		return true
	}

	if rl.config.OnLimit != nil {
		rl.config.OnLimit(rl.config.Name)
	}

	return false
}

// Wait blocks until a request is allowed or context is cancelled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	return rl.WaitN(ctx, 1)
}

// WaitN blocks until n requests are allowed or context is cancelled.
func (rl *RateLimiter) WaitN(ctx context.Context, n int) error {
	// Try immediate allow
	if rl.AllowN(n) {
		return nil
	}

	// Calculate wait time
	waitTime := rl.reserveN(n)
	if waitTime <= 0 {
		return nil
	}

	timer := time.NewTimer(waitTime)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Execute runs a function if rate limit allows.
func (rl *RateLimiter) Execute(fn func() error) error {
	if !rl.Allow() {
		return ErrRateLimited
	}
	return fn()
}

// ExecuteWait blocks until rate limit allows, then runs the function.
func (rl *RateLimiter) ExecuteWait(ctx context.Context, fn func() error) error {
	if err := rl.Wait(ctx); err != nil {
		return err
	}
	return fn()
}

// refill adds tokens based on time elapsed.
func (rl *RateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill).Seconds()
	rl.lastRefill = now

	// Add tokens based on elapsed time
	rl.tokens += elapsed * rl.config.Rate

	// Cap at burst size
	if rl.tokens > float64(rl.config.Burst) {
		rl.tokens = float64(rl.config.Burst)
	}
}

// reserveN reserves n tokens and returns the wait time.
func (rl *RateLimiter) reserveN(n int) time.Duration {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.refill()

	// If we have enough tokens, consume them
	if rl.tokens >= float64(n) {
		rl.tokens -= float64(n)
		return 0
	}

	// Calculate how long to wait for tokens
	needed := float64(n) - rl.tokens
	waitSeconds := needed / rl.config.Rate

	// Reserve the tokens
	rl.tokens -= float64(n)

	return time.Duration(waitSeconds * float64(time.Second))
}

// Tokens returns the current number of available tokens.
func (rl *RateLimiter) Tokens() float64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.refill()
	return rl.tokens
}

// Rate returns the rate limit (requests per second).
func (rl *RateLimiter) Rate() float64 {
	return rl.config.Rate
}

// Burst returns the burst size.
func (rl *RateLimiter) Burst() int {
	return rl.config.Burst
}
