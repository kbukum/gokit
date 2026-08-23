package resilience

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	apperr "github.com/kbukum/gokit/errors"
)

func TestRetryConfigValidateGuards(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cfg     RetryConfig
		wantErr bool
	}{
		{"valid", DefaultRetryConfig(), false},
		{"zero_attempts", RetryConfig{MaxAttempts: 0}, true},
		{"negative_initial", RetryConfig{MaxAttempts: 1, InitialBackoff: -1}, true},
		{"max_lt_initial", RetryConfig{MaxAttempts: 1, InitialBackoff: 2 * time.Second, MaxBackoff: time.Second}, true},
		{"jitter_out_of_range", RetryConfig{MaxAttempts: 1, Jitter: 1.5}, true},
		{"jitter_nan", RetryConfig{MaxAttempts: 1, Jitter: math.NaN()}, true},
		{"backoff_factor_inf", RetryConfig{MaxAttempts: 1, BackoffFactor: math.Inf(1)}, true},
		{"negative_budget", RetryConfig{MaxAttempts: 1, MaxElapsedTime: -1}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr != (err != nil) {
				t.Fatalf("Validate err=%v, wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr && !apperr.IsAppError(err) {
				t.Fatalf("expected typed AppError, got %T", err)
			}
		})
	}
}

func TestConfigValidateGuards(t *testing.T) {
	t.Parallel()
	if err := DefaultCircuitBreakerConfig("cb").Validate(); err != nil {
		t.Fatalf("default cb invalid: %v", err)
	}
	if err := (CircuitBreakerConfig{MaxFailures: 0}).Validate(); err == nil {
		t.Fatal("expected cb guard error")
	}
	if err := DefaultBulkheadConfig("b").Validate(); err != nil {
		t.Fatalf("default bulkhead invalid: %v", err)
	}
	if err := (BulkheadConfig{MaxConcurrent: 0}).Validate(); err == nil {
		t.Fatal("expected bulkhead guard error")
	}
	if err := DefaultRateLimiterConfig("r").Validate(); err != nil {
		t.Fatalf("default rate limiter invalid: %v", err)
	}
	if err := (RateLimiterConfig{Rate: 0, Burst: 1}).Validate(); err == nil {
		t.Fatal("expected rate limiter guard error")
	}
	if err := (RateLimiterConfig{Rate: math.NaN(), Burst: 1}).Validate(); err == nil {
		t.Fatal("expected rate limiter NaN guard error")
	}
	if err := (RateLimiterConfig{Rate: math.Inf(1), Burst: 1}).Validate(); err == nil {
		t.Fatal("expected rate limiter Inf guard error")
	}
}

func TestRetryHonorsElapsedBudget(t *testing.T) {
	t.Parallel()
	var attempts int
	cfg := RetryConfig{
		MaxAttempts:    100,
		Strategy:       ConstantBackoff,
		InitialBackoff: 40 * time.Millisecond,
		MaxBackoff:     40 * time.Millisecond,
		MaxElapsedTime: 50 * time.Millisecond,
		RetryIf:        func(error) bool { return true },
	}
	sentinel := errors.New("fail")
	start := time.Now()
	_, err := Retry(context.Background(), cfg, func() (int, error) {
		attempts++
		return 0, sentinel
	})
	elapsed := time.Since(start)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	// The budget should stop retries well before MaxAttempts (100) is reached.
	if attempts >= 100 {
		t.Fatalf("attempts = %d, expected budget to short-circuit", attempts)
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("elapsed = %v, budget not honored", elapsed)
	}
}

func TestRetryPresets(t *testing.T) {
	t.Parallel()
	for _, p := range []RetryPreset{RetryFast, RetryStandard, RetryExternalService} {
		if err := p.Config().Validate(); err != nil {
			t.Fatalf("preset %v invalid: %v", p, err)
		}
		if p.Config().MaxElapsedTime <= 0 {
			t.Fatalf("preset %v missing elapsed budget", p)
		}
	}
	if RetryFast.Config().MaxAttempts != 2 {
		t.Fatalf("fast attempts = %d, want 2", RetryFast.Config().MaxAttempts)
	}
}
