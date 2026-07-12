package resilience

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  10.0,
		Burst: 5,
	})

	// Should allow burst size requests immediately
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("request %d should be allowed", i)
		}
	}
}

func TestRateLimiter_RejectsOverLimit(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  10.0,
		Burst: 3,
	})

	// Exhaust burst
	for i := 0; i < 3; i++ {
		rl.Allow()
	}

	// Next request should be rejected
	if rl.Allow() {
		t.Error("request should be rejected over burst limit")
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  100.0, // 100 per second = 1 per 10ms
		Burst: 1,
	})

	// Exhaust the single token
	if !rl.Allow() {
		t.Error("first request should be allowed")
	}

	// Should be rejected
	if rl.Allow() {
		t.Error("second request should be rejected")
	}

	// Wait for refill
	time.Sleep(20 * time.Millisecond)

	// Should be allowed again
	if !rl.Allow() {
		t.Error("request after refill should be allowed")
	}
}

func TestRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  100.0,
		Burst: 1,
	})

	// Exhaust burst
	rl.Allow()

	start := time.Now()
	err := rl.Wait(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Should have waited about 10ms for 1 token at 100/s
	if elapsed < 5*time.Millisecond || elapsed > 50*time.Millisecond {
		t.Errorf("expected wait around 10ms, got %v", elapsed)
	}
}

func TestRateLimiter_WaitRespectsContext(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  1.0, // 1 per second - slow
		Burst: 1,
	})

	// Exhaust burst
	rl.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestRateLimiter_Execute(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  10.0,
		Burst: 1,
	})

	// First execute should succeed
	called := false
	err := rl.Execute(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if !called {
		t.Error("function was not called")
	}

	// Second should be rejected
	err = rl.Execute(func() error {
		return nil
	})

	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestRateLimiter_OnLimitCallback(t *testing.T) {
	var limitCount int32

	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  10.0,
		Burst: 1,
		OnLimit: func(name string) {
			atomic.AddInt32(&limitCount, 1)
		},
	})

	// Exhaust burst
	rl.Allow()

	// Trigger limit
	rl.Allow()
	rl.Allow()

	if limitCount != 2 {
		t.Errorf("expected 2 limit callbacks, got %d", limitCount)
	}
}

func TestRateLimiter_AllowN(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  10.0,
		Burst: 5,
	})

	// Should allow 5 at once
	if !rl.AllowN(5) {
		t.Error("should allow 5 requests")
	}

	// Should reject next
	if rl.Allow() {
		t.Error("should reject after burst exhausted")
	}
}

func TestRateLimiter_Tokens(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  10.0,
		Burst: 5,
	})

	initialTokens := rl.Tokens()
	if initialTokens < 4.9 || initialTokens > 5.1 {
		t.Errorf("expected ~5 tokens, got %f", initialTokens)
	}

	rl.AllowN(3)

	tokens := rl.Tokens()
	// Use approximate comparison due to time-based refill adding small amounts
	if tokens < 1.9 || tokens > 2.5 {
		t.Errorf("expected ~2 tokens, got %f", tokens)
	}
}

func TestRateLimiter_RateAndBurst(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "test",
		Rate:  42.0,
		Burst: 100,
	})

	if rl.Rate() != 42.0 {
		t.Errorf("expected rate 42, got %f", rl.Rate())
	}

	if rl.Burst() != 100 {
		t.Errorf("expected burst 100, got %d", rl.Burst())
	}
}

func TestDefaultRateLimiterConfig(t *testing.T) {
	cfg := DefaultRateLimiterConfig("svc")
	if cfg.Name != "svc" {
		t.Errorf("expected Name svc, got %q", cfg.Name)
	}
	if cfg.Rate != 10.0 {
		t.Errorf("expected Rate 10.0, got %v", cfg.Rate)
	}
	if cfg.Burst != 20 {
		t.Errorf("expected Burst 20, got %d", cfg.Burst)
	}
}

func TestRateLimiter_ExecuteWait(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{Name: "t", Rate: 100, Burst: 1})
	called := false
	if err := rl.ExecuteWait(context.Background(), func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected function to be called")
	}
}

func TestRateLimiter_ExecuteWaitCanceledContext(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{Name: "t", Rate: 1, Burst: 1})
	rl.AllowN(1) // drain the bucket so Wait must block
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := rl.ExecuteWait(ctx, func() error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if called {
		t.Fatal("function should not run when Wait fails")
	}
}

func TestRateLimiter_WaitNBlocksThenAllows(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{Name: "t", Rate: 1000, Burst: 1})
	rl.AllowN(1) // drain so WaitN must reserve and wait
	if err := rl.WaitN(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
