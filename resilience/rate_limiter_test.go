package resilience

import (
	"context"
	"errors"
	"sync"
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

func TestRateLimiter_AllowNExceedsBurst(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "exceed",
		Rate:  100,
		Burst: 5,
	})

	// n > Burst should always be false.
	if rl.AllowN(6) {
		t.Error("AllowN(6) with Burst=5 should be false")
	}
	if rl.AllowN(100) {
		t.Error("AllowN(100) with Burst=5 should be false")
	}
}

func TestRateLimiter_TokenRefillRateAccuracy(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "refill",
		Rate:  100.0, // 100 tokens/sec → 1 token per 10ms
		Burst: 10,
	})

	// Exhaust all tokens.
	rl.AllowN(10)

	// Wait 50ms → expect ~5 tokens refilled.
	time.Sleep(55 * time.Millisecond)

	tokens := rl.Tokens()
	if tokens < 3 || tokens > 8 {
		t.Errorf("expected ~5 tokens after 50ms at rate 100/s, got %.2f", tokens)
	}
}

func TestRateLimiter_WaitCancellation(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "cancel",
		Rate:  1.0, // Very slow: 1 token/sec
		Burst: 1,
	})

	rl.Allow() // Exhaust

	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := rl.Wait(ctx)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("Wait should return promptly on cancel, took %v", elapsed)
	}
}

func TestRateLimiter_ExecuteWaitUnderContention(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "contention",
		Rate:  200.0, // 200 tokens/sec
		Burst: 5,
	})

	var completed int32
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := rl.ExecuteWait(ctx, func() error {
				atomic.AddInt32(&completed, 1)
				return nil
			})
			if err != nil && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if c := atomic.LoadInt32(&completed); c == 0 {
		t.Error("expected some goroutines to complete")
	}
}

func TestRateLimiter_OnLimitCallbackVerification(t *testing.T) {
	t.Parallel()
	var limitedCount int32

	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "cb-verify",
		Rate:  100.0,
		Burst: 2,
		OnLimit: func(name string) {
			if name != "cb-verify" {
				return
			}
			atomic.AddInt32(&limitedCount, 1)
		},
	})

	// Exhaust burst.
	rl.Allow()
	rl.Allow()

	// These should trigger OnLimit.
	rl.Allow()
	rl.Allow()
	rl.Allow()

	if atomic.LoadInt32(&limitedCount) != 3 {
		t.Errorf("expected 3 OnLimit calls, got %d", limitedCount)
	}
}

func TestRateLimiter_ZeroRateDefaultsToTen(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "zero-rate",
		Rate:  0,
		Burst: 0,
	})

	// With default Rate=10 and default Burst=int(Rate)=10, should allow 10.
	allowed := 0
	for i := 0; i < 15; i++ {
		if rl.Allow() {
			allowed++
		}
	}
	if allowed != 10 {
		t.Errorf("expected 10 allowed with default rate, got %d", allowed)
	}
}

func TestRateLimiter_BurstExhaustionThenRefill(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "burst-refill",
		Rate:  100.0,
		Burst: 3,
	})

	// Exhaust all burst tokens.
	for i := 0; i < 3; i++ {
		if !rl.Allow() {
			t.Fatalf("Allow() should succeed for token %d", i)
		}
	}
	if rl.Allow() {
		t.Error("should be rejected after burst exhaustion")
	}

	// Wait enough for at least 1 token refill (10ms at 100/s).
	time.Sleep(15 * time.Millisecond)
	if !rl.Allow() {
		t.Error("should allow after refill time")
	}
}

func TestRateLimiter_AllowVsAllowNConsistency(t *testing.T) {
	t.Parallel()
	rl1 := NewRateLimiter(RateLimiterConfig{Name: "a1", Rate: 100, Burst: 5})
	rl2 := NewRateLimiter(RateLimiterConfig{Name: "a2", Rate: 100, Burst: 5})

	// AllowN(1) five times on rl1.
	var count1 int
	for i := 0; i < 10; i++ {
		if rl1.AllowN(1) {
			count1++
		}
	}

	// Allow() five times on rl2.
	var count2 int
	for i := 0; i < 10; i++ {
		if rl2.Allow() {
			count2++
		}
	}

	if count1 != count2 {
		t.Errorf("Allow and AllowN(1) should behave identically: got %d vs %d", count1, count2)
	}
}
