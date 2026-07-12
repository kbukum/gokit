package resilience

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"
)

// Common retry errors.
var (
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
)

// BackoffStrategy defines how retry delays grow between attempts.
type BackoffStrategy int

const (
	// ExponentialBackoff grows delays geometrically using BackoffFactor.
	ExponentialBackoff BackoffStrategy = iota
	// ConstantBackoff uses the same delay for every retry.
	ConstantBackoff
	// LinearBackoff increases delay linearly by InitialBackoff each retry.
	LinearBackoff
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the first).
	MaxAttempts int
	// InitialBackoff is the initial delay between retries.
	InitialBackoff time.Duration
	// MaxBackoff is the maximum delay between retries.
	MaxBackoff time.Duration
	// Strategy controls how the delay grows between retries.
	Strategy BackoffStrategy
	// BackoffFactor is the multiplier for exponential backoff.
	BackoffFactor float64
	// Jitter adds randomness to backoff (0.0 to 1.0).
	Jitter float64
	// Rand supplies a uniform random float64 in [0.0, 1.0) used to compute
	// jitter. Leave nil for the concurrency-safe, auto-seeded default; inject a
	// seeded source (e.g. rand.New(rand.NewPCG(seed1, seed2)).Float64) to make
	// backoff deterministic under test.
	Rand func() float64
	// RetryIf determines if an error should be retried.
	RetryIf func(error) bool
	// OnRetry is called before each retry.
	OnRetry func(attempt int, err error, backoff time.Duration)
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		Strategy:       ExponentialBackoff,
		BackoffFactor:  2.0,
		Jitter:         0.1,
		RetryIf:        DefaultRetryIf,
	}
}

// DefaultRetryIf retries all errors except context cancellation.
func DefaultRetryIf(err error) bool {
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// Retry executes a function with retry logic.
// Returns the result of the function or the last error if all retries fail.
func Retry[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error

	cfg = normalizeRetryConfig(cfg)

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Check context before each attempt
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if we should retry
		if !cfg.RetryIf(err) {
			return zero, err
		}

		// Don't sleep after the last attempt
		if attempt == cfg.MaxAttempts {
			break
		}

		backoff := calculateBackoff(attempt, cfg)

		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt, err, backoff)
		}

		// Wait with context awareness
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}

	return zero, lastErr
}

// RetryFunc executes a function that returns only an error.
func RetryFunc(ctx context.Context, cfg RetryConfig, fn func() error) error {
	_, err := Retry(ctx, cfg, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// BackoffDelay returns the normalized retry delay for a failed attempt.
// attempt is one-based and matches the attempt value passed to RetryConfig.OnRetry.
// If attempt < 1, it is clamped to 1.
func BackoffDelay(attempt int, cfg RetryConfig) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return calculateBackoff(attempt, cfg)
}

func normalizeRetryConfig(cfg RetryConfig) RetryConfig {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 10 * time.Second
	}
	if cfg.BackoffFactor <= 0 {
		cfg.BackoffFactor = 2.0
	}
	if cfg.RetryIf == nil {
		cfg.RetryIf = DefaultRetryIf
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Float64
	}
	return cfg
}

// calculateBackoff calculates the backoff duration for an attempt.
func calculateBackoff(attempt int, cfg RetryConfig) time.Duration {
	cfg = normalizeRetryConfig(cfg)

	var backoffFloat float64
	switch cfg.Strategy {
	case ConstantBackoff:
		backoffFloat = float64(cfg.InitialBackoff)
	case LinearBackoff:
		backoffFloat = float64(cfg.InitialBackoff) * float64(attempt)
	default:
		backoffFloat = float64(cfg.InitialBackoff) * math.Pow(cfg.BackoffFactor, float64(attempt-1))
	}

	if cfg.Jitter > 0 {
		jitterRange := backoffFloat * cfg.Jitter
		jitter := (cfg.Rand()*2 - 1) * jitterRange
		backoffFloat += jitter
	}

	if backoffFloat > float64(cfg.MaxBackoff) {
		backoffFloat = float64(cfg.MaxBackoff)
	}
	if backoffFloat < 0 {
		backoffFloat = float64(cfg.InitialBackoff)
	}

	return time.Duration(backoffFloat)
}

// RetryWithBackoff is a convenience function for simple retry with exponential backoff.
func RetryWithBackoff[T any](ctx context.Context, maxAttempts int, fn func() (T, error)) (T, error) {
	return Retry(ctx, RetryConfig{
		MaxAttempts:    maxAttempts,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		Strategy:       ExponentialBackoff,
		BackoffFactor:  2.0,
		Jitter:         0.1,
	}, fn)
}
