package resilience

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIntegration_CBPlusRetry(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:        "cb-retry",
		MaxFailures: 3,
		Timeout:     time.Second,
	})

	cfg := RetryConfig{
		MaxAttempts:    5,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  1.0,
	}

	callCount := 0
	_, err := Retry(context.Background(), cfg, func() (int, error) {
		callCount++
		return 0, cb.Execute(func() error {
			return errors.New("service down")
		})
	})

	// After 3 failures the CB opens. Attempts 4 and 5 should fail fast with
	// ErrCircuitOpen, not even calling the inner fn.
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen from composed retry+CB, got %v", err)
	}
	if callCount != 5 {
		t.Errorf("expected 5 retry attempts, got %d", callCount)
	}
}

func TestIntegration_BulkheadPlusRateLimiter(t *testing.T) {
	bh := NewBulkhead(BulkheadConfig{
		Name:          "bh-rl",
		MaxConcurrent: 3,
		MaxWait:       50 * time.Millisecond,
	})
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "bh-rl",
		Rate:  1000.0,
		Burst: 5,
	})

	var completed int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := bh.Execute(context.Background(), func() error {
				return rl.Execute(func() error {
					atomic.AddInt32(&completed, 1)
					time.Sleep(5 * time.Millisecond)
					return nil
				})
			})
			// Either succeeds, or hits bulkhead/rate limit.
			if err != nil &&
				!errors.Is(err, ErrBulkheadFull) &&
				!errors.Is(err, ErrBulkheadTimeout) &&
				!errors.Is(err, ErrRateLimited) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if c := atomic.LoadInt32(&completed); c == 0 {
		t.Error("expected some requests to complete through both guards")
	}
}

func TestIntegration_AllFourPatterns(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:        "all4",
		MaxFailures: 10,
		Timeout:     time.Second,
	})
	bh := NewBulkhead(BulkheadConfig{
		Name:          "all4",
		MaxConcurrent: 5,
		MaxWait:       100 * time.Millisecond,
	})
	rl := NewRateLimiter(RateLimiterConfig{
		Name:  "all4",
		Rate:  500.0,
		Burst: 10,
	})
	retryCfg := RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  1.0,
	}

	ctx := context.Background()
	callCount := 0

	err := RetryFunc(ctx, retryCfg, func() error {
		return cb.Execute(func() error {
			return bh.Execute(ctx, func() error {
				return rl.ExecuteWait(ctx, func() error {
					callCount++
					return nil
				})
			})
		})
	})
	if err != nil {
		t.Errorf("expected success through all four patterns, got %v", err)
	}
	if callCount < 1 {
		t.Error("expected at least one successful call")
	}
}

func TestIntegration_DegradationManagerTracksCBDuringLoad(t *testing.T) {
	dm := NewDegradationManager()
	config := CircuitBreakerConfig{
		Name:             "load-svc",
		MaxFailures:      5,
		Timeout:          15 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		OnStateChange:    dm.OnCircuitBreakerStateChange("load-svc"),
	}
	cb := NewCircuitBreaker(config)

	// Generate load that trips the CB.
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cb.Execute(func() error { return errors.New("fail") })
		}()
	}
	wg.Wait()

	// CB should be open, DM should track it as unhealthy.
	if dm.ServiceStatus("load-svc").Health != Unhealthy {
		t.Errorf("expected Unhealthy after load, got %s", dm.ServiceStatus("load-svc").Health)
	}

	// Wait for half-open.
	time.Sleep(20 * time.Millisecond)
	_ = cb.State() // trigger check

	health := dm.ServiceStatus("load-svc").Health
	if health != Degraded {
		t.Errorf("expected Degraded in half-open, got %s", health)
	}
}

func TestIntegration_RecoveryScenario(t *testing.T) {
	dm := NewDegradationManager()
	config := CircuitBreakerConfig{
		Name:             "recover",
		MaxFailures:      2,
		Timeout:          15 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		OnStateChange:    dm.OnCircuitBreakerStateChange("recover"),
	}
	cb := NewCircuitBreaker(config)

	// Phase 1: System healthy.
	_ = cb.Execute(func() error { return nil })
	// Service might not yet be tracked by DM (no state change happened).
	// IsHealthy returns true when no services are degraded — that's fine.

	// Phase 2: Service degrades.
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}
	if dm.ServiceStatus("recover").Health != Unhealthy {
		t.Errorf("expected Unhealthy, got %s", dm.ServiceStatus("recover").Health)
	}
	if dm.IsHealthy() {
		t.Error("system should not be healthy when service is unhealthy")
	}

	// Phase 3: Wait for half-open, then recover.
	time.Sleep(20 * time.Millisecond)
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("expected successful execute in half-open, got %v", err)
	}

	// Phase 4: Fully operational.
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after recovery, got %s", cb.State())
	}
	if dm.ServiceStatus("recover").Health != Healthy {
		t.Errorf("expected Healthy after recovery, got %s", dm.ServiceStatus("recover").Health)
	}
	if !dm.IsHealthy() {
		t.Error("system should be fully healthy after recovery")
	}
}
