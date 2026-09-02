package resilience

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"time"

	apperr "github.com/kbukum/gokit/errors"
)

// Common retry errors. ErrMaxRetriesExceeded is a typed AppError so callers can
// branch on the error code, while errors.Is still matches the sentinel.
var (
	ErrMaxRetriesExceeded = apperr.New(apperr.ErrCodeServiceUnavailable, "max retries exceeded", http.StatusServiceUnavailable)
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

// String returns the lower-case wire name of the strategy ("exponential",
// "constant", "linear"). An unrecognized value is rendered in an identifying
// form (e.g. "BackoffStrategy(7)") rather than masquerading as a valid strategy.
func (s BackoffStrategy) String() string {
	switch s {
	case ExponentialBackoff:
		return "exponential"
	case ConstantBackoff:
		return "constant"
	case LinearBackoff:
		return "linear"
	default:
		return fmt.Sprintf("BackoffStrategy(%d)", int(s))
	}
}

func (s BackoffStrategy) valid() bool {
	switch s {
	case ExponentialBackoff, ConstantBackoff, LinearBackoff:
		return true
	default:
		return false
	}
}

// MarshalText encodes the strategy as its lower-case wire name so it serializes
// to a stable string in JSON, YAML, and other text formats. An unrecognized
// value is rejected rather than silently coerced to a default, so a corrupt or
// programmer-introduced strategy surfaces instead of round-tripping incorrectly.
func (s BackoffStrategy) MarshalText() ([]byte, error) {
	if !s.valid() {
		return nil, fmt.Errorf("resilience: unknown backoff strategy %d", int(s))
	}
	return []byte(s.String()), nil
}

// UnmarshalText decodes the lower-case wire name back into a BackoffStrategy,
// letting config keys and JSON use "exponential"/"constant"/"linear" rather
// than opaque integers. An empty value decodes to the exponential default.
func (s *BackoffStrategy) UnmarshalText(text []byte) error {
	switch string(text) {
	case "", "exponential":
		*s = ExponentialBackoff
	case "constant":
		*s = ConstantBackoff
	case "linear":
		*s = LinearBackoff
	default:
		return fmt.Errorf("resilience: unknown backoff strategy %q", text)
	}
	return nil
}

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the first).
	MaxAttempts int `json:"max_attempts,omitempty" yaml:"max_attempts" mapstructure:"max_attempts"`
	// InitialBackoff is the initial delay between retries.
	InitialBackoff time.Duration `json:"initial_backoff,omitempty" yaml:"initial_backoff" mapstructure:"initial_backoff"`
	// MaxBackoff is the maximum delay between retries.
	MaxBackoff time.Duration `json:"max_backoff,omitempty" yaml:"max_backoff" mapstructure:"max_backoff"`
	// MaxElapsedTime bounds the elapsed-time budget for retries. It is enforced at each
	// attempt boundary (the loop stops before starting an attempt or a backoff sleep once
	// the budget is spent) and returns the last attempt's error. Zero means unbounded (only
	// MaxAttempts bounds retries). A single in-flight attempt is not interrupted, because
	// the retried function receives no context; supply a context-aware function and derive a
	// per-attempt deadline yourself if you need to cancel a running attempt.
	MaxElapsedTime time.Duration `json:"max_elapsed_time,omitempty" yaml:"max_elapsed_time" mapstructure:"max_elapsed_time"`
	// Strategy controls how the delay grows between retries.
	Strategy BackoffStrategy `json:"strategy,omitempty" yaml:"strategy" mapstructure:"strategy"`
	// BackoffFactor is the multiplier for exponential backoff.
	BackoffFactor float64 `json:"backoff_factor,omitempty" yaml:"backoff_factor" mapstructure:"backoff_factor"`
	// Jitter adds randomness to backoff (0.0 to 1.0).
	Jitter float64 `json:"jitter,omitempty" yaml:"jitter" mapstructure:"jitter"`
	// Rand supplies a uniform random float64 in [0.0, 1.0) used to compute jitter.
	// Leave nil for the concurrency-safe, auto-seeded default;
	// inject a seeded source (e.g. rand.New(rand.NewPCG(seed1, seed2)).Float64) to make backoff deterministic under test.
	Rand func() float64 `json:"-" yaml:"-" mapstructure:"-"`
	// RetryIf determines if an error should be retried.
	RetryIf func(error) bool `json:"-" yaml:"-" mapstructure:"-"`
	// OnRetry is called before each retry.
	OnRetry func(attempt int, err error, backoff time.Duration) `json:"-" yaml:"-" mapstructure:"-"`
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

// Validate rejects an inconsistent retry configuration. Unlike the normalizing
// constructors (which silently fill zero fields with defaults), Validate is the
// explicit guard callers use to fail closed on invalid values before running work.
func (c RetryConfig) Validate() error {
	if c.MaxAttempts < 1 {
		return apperr.InvalidInput("max_attempts", "must be at least 1")
	}
	if c.InitialBackoff < 0 {
		return apperr.InvalidInput("initial_backoff", "must not be negative")
	}
	if c.MaxBackoff < 0 {
		return apperr.InvalidInput("max_backoff", "must not be negative")
	}
	if c.MaxBackoff > 0 && c.InitialBackoff > c.MaxBackoff {
		return apperr.InvalidInput("max_backoff", "must be >= initial_backoff")
	}
	if c.BackoffFactor < 0 || math.IsNaN(c.BackoffFactor) || math.IsInf(c.BackoffFactor, 0) {
		return apperr.InvalidInput("backoff_factor", "must be a finite non-negative number")
	}
	if c.Jitter < 0 || c.Jitter > 1 || math.IsNaN(c.Jitter) {
		return apperr.InvalidInput("jitter", "must be a finite number within [0.0, 1.0]")
	}
	if c.MaxElapsedTime < 0 {
		return apperr.InvalidInput("max_elapsed_time", "must not be negative")
	}
	return nil
}

// DefaultRetryIf retries all errors except context cancellation.
func DefaultRetryIf(err error) bool {
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

// Retry executes a function with retry logic. On success it returns the result.
// When a non-retryable error is encountered it returns that error unchanged.
// When all attempts (or the elapsed-time budget) are exhausted it returns
// ErrMaxRetriesExceeded with the last error preserved as the cause, so callers
// can branch on errors.Is(err, ErrMaxRetriesExceeded) while still unwrapping the
// underlying failure.
func Retry[T any](ctx context.Context, cfg RetryConfig, fn func() (T, error)) (T, error) {
	var zero T
	var lastErr error

	cfg = normalizeRetryConfig(cfg)
	start := time.Now()

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		// Stop before another attempt once the elapsed-time budget is spent, so a slow
		// prior attempt cannot buy additional attempts beyond the budget.
		if cfg.MaxElapsedTime > 0 && attempt > 1 && time.Since(start) >= cfg.MaxElapsedTime {
			break
		}

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

		// Refuse to sleep past the total elapsed-time budget. Compare the backoff against
		// the remaining budget rather than elapsed+backoff, so a large configured backoff
		// cannot overflow the duration addition and wrap negative past the guard. Using >=
		// also stops when the sleep would land exactly on the deadline, leaving no room for
		// the next attempt to run.
		if cfg.MaxElapsedTime > 0 && backoff >= cfg.MaxElapsedTime-time.Since(start) {
			break
		}

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

	return zero, ErrMaxRetriesExceeded.WithCause(lastErr)
}

// RetryFunc executes a function that returns only an error.
func RetryFunc(ctx context.Context, cfg RetryConfig, fn func() error) error {
	_, err := Retry(ctx, cfg, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// BackoffDelay returns the normalized retry delay for a failed attempt. attempt is one-based
// and matches the attempt value passed to RetryConfig.OnRetry. If attempt < 1, it is clamped to 1.
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
