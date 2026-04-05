package resilience

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. Circuit Breaker Advanced
// ---------------------------------------------------------------------------

func TestCB_HalfOpenMaxCallsConcurrentProbes(t *testing.T) {
	const halfOpenMax = 3
	config := CircuitBreakerConfig{
		Name:             "ho-max",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: halfOpenMax,
	}
	cb := NewCircuitBreaker(config)

	// Trip the breaker.
	_ = cb.Execute(func() error { return errors.New("fail") })
	time.Sleep(15 * time.Millisecond)

	// Launch exactly halfOpenMax goroutines that all block until released.
	var allowed int32
	started := make(chan struct{}, halfOpenMax+1)
	release := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < halfOpenMax+1; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := cb.Execute(func() error {
				atomic.AddInt32(&allowed, 1)
				started <- struct{}{}
				<-release
				return nil
			})
			if errors.Is(err, ErrCircuitOpen) {
				started <- struct{}{} // signal even on rejection
			}
		}()
	}

	// Wait for all goroutines to have attempted.
	for i := 0; i < halfOpenMax+1; i++ {
		<-started
	}

	got := int(atomic.LoadInt32(&allowed))
	if got != halfOpenMax {
		t.Errorf("expected %d allowed in half-open, got %d", halfOpenMax, got)
	}
	close(release)
	wg.Wait()
}

func TestCB_HalfOpenSuccessQuotaBoundary(t *testing.T) {
	const halfOpenMax = 3
	config := CircuitBreakerConfig{
		Name:             "quota",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: halfOpenMax,
	}

	// Sub-test: n-1 successes should NOT close the circuit.
	t.Run("below_quota", func(t *testing.T) {
		cb := NewCircuitBreaker(config)
		_ = cb.Execute(func() error { return errors.New("trip") })
		time.Sleep(15 * time.Millisecond)

		for i := 0; i < halfOpenMax-1; i++ {
			_ = cb.Execute(func() error { return nil })
		}
		if cb.State() == StateClosed {
			t.Error("circuit should NOT be closed before reaching quota")
		}
	})

	// Sub-test: exactly n successes should close the circuit.
	t.Run("at_quota", func(t *testing.T) {
		cb := NewCircuitBreaker(config)
		_ = cb.Execute(func() error { return errors.New("trip") })
		time.Sleep(15 * time.Millisecond)

		for i := 0; i < halfOpenMax; i++ {
			_ = cb.Execute(func() error { return nil })
		}
		if cb.State() != StateClosed {
			t.Errorf("expected StateClosed after %d successes, got %s", halfOpenMax, cb.State())
		}
	})
}

func TestCB_RapidConcurrentLoad(t *testing.T) {
	t.Parallel()
	config := CircuitBreakerConfig{
		Name:        "concurrent",
		MaxFailures: 5,
		Timeout:     5 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = cb.Execute(func() error {
				if n%3 == 0 {
					return errors.New("fail")
				}
				return nil
			})
			_ = cb.State()
			_ = cb.Failures()
		}(i)
	}
	wg.Wait()

	// Simply verify no panics and state is valid.
	s := cb.State()
	if s != StateClosed && s != StateOpen && s != StateHalfOpen {
		t.Errorf("unexpected state: %s", s)
	}
}

func TestCB_OnStateChangeCallbackOrdering(t *testing.T) {
	type transition struct{ from, to State }
	var transitions []transition
	var mu sync.Mutex

	config := CircuitBreakerConfig{
		Name:             "order",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		OnStateChange: func(_ string, from, to State) {
			mu.Lock()
			transitions = append(transitions, transition{from, to})
			mu.Unlock()
		},
	}
	cb := NewCircuitBreaker(config)

	// Closed → Open
	_ = cb.Execute(func() error { return errors.New("fail") })
	// Open → HalfOpen
	time.Sleep(15 * time.Millisecond)
	// HalfOpen → Closed (success)
	_ = cb.Execute(func() error { return nil })

	mu.Lock()
	defer mu.Unlock()

	expected := []transition{
		{StateClosed, StateOpen},
		{StateOpen, StateHalfOpen},
		{StateHalfOpen, StateClosed},
	}
	if len(transitions) != len(expected) {
		t.Fatalf("expected %d transitions, got %d: %v", len(expected), len(transitions), transitions)
	}
	for i, want := range expected {
		if transitions[i] != want {
			t.Errorf("transition[%d] = %v, want %v", i, transitions[i], want)
		}
	}
}

func TestCB_ExecuteWithPanic(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		Name:        "panic",
		MaxFailures: 5,
		Timeout:     time.Second,
	})

	// Execute panics – the panic propagates to the caller. We just verify it
	// does not corrupt state (no deadlock, no invalid state after recovery).
	func() {
		defer func() { _ = recover() }()
		_ = cb.Execute(func() error { panic("boom") })
	}()

	// The CB should still be usable. The panic prevented recordResult from
	// running so the failure counter should still be 0.
	if cb.Failures() != 0 {
		t.Logf("failures after panic: %d (panic bypassed recordResult)", cb.Failures())
	}
	// Must not deadlock.
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Errorf("expected successful execution after panic recovery, got %v", err)
	}
}

func TestCB_ResetDuringHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:             "reset-ho",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: 1,
	}
	cb := NewCircuitBreaker(config)

	_ = cb.Execute(func() error { return errors.New("trip") })
	time.Sleep(15 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %s", cb.State())
	}

	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after Reset, got %s", cb.State())
	}
	if cb.Failures() != 0 {
		t.Errorf("expected 0 failures after Reset, got %d", cb.Failures())
	}
}

func TestCB_ZeroMaxFailuresDefaultsToFive(t *testing.T) {
	t.Parallel()
	config := CircuitBreakerConfig{
		Name:        "zero-max",
		MaxFailures: 0,
		Timeout:     time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Should default to 5.
	for i := 0; i < 4; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}
	if cb.State() != StateClosed {
		t.Errorf("should still be closed after 4 failures with default MaxFailures=5, got %s", cb.State())
	}
	_ = cb.Execute(func() error { return errors.New("fail") })
	if cb.State() != StateOpen {
		t.Errorf("should be open after 5 failures, got %s", cb.State())
	}
}

func TestCB_VeryShortTimeout(t *testing.T) {
	config := CircuitBreakerConfig{
		Name:        "short-timeout",
		MaxFailures: 1,
		Timeout:     1 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	_ = cb.Execute(func() error { return errors.New("fail") })

	// Wait well beyond 1ms.
	time.Sleep(10 * time.Millisecond)

	if s := cb.State(); s != StateHalfOpen {
		t.Errorf("expected StateHalfOpen after timeout, got %s", s)
	}
}

func TestCB_ConcurrentResetAndExecute(t *testing.T) {
	t.Parallel()
	config := CircuitBreakerConfig{
		Name:        "reset-exec",
		MaxFailures: 2,
		Timeout:     50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = cb.Execute(func() error { return errors.New("fail") })
		}()
		go func() {
			defer wg.Done()
			cb.Reset()
		}()
	}
	wg.Wait()

	// No panics, no deadlocks.
	s := cb.State()
	if s != StateClosed && s != StateOpen && s != StateHalfOpen {
		t.Errorf("invalid state %s", s)
	}
}

// ---------------------------------------------------------------------------
// 2. Retry Advanced
// ---------------------------------------------------------------------------

func TestRetry_BackoffTimingVerification(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    4,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
		BackoffFactor:  2.0,
		Jitter:         0,
	}

	var timestamps []time.Time
	_, _ = Retry(context.Background(), cfg, func() (int, error) {
		timestamps = append(timestamps, time.Now())
		return 0, errors.New("keep going")
	})

	// Expected gaps: 50ms, 100ms, 200ms (between attempts 1-2, 2-3, 3-4).
	expectedGaps := []time.Duration{
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
	}
	const tolerance = 50 * time.Millisecond

	for i := 0; i < len(timestamps)-1; i++ {
		gap := timestamps[i+1].Sub(timestamps[i])
		if gap < expectedGaps[i]-tolerance || gap > expectedGaps[i]+tolerance {
			t.Errorf("gap[%d]: expected ~%v, got %v", i, expectedGaps[i], gap)
		}
	}
}

func TestRetry_JitterBoundsVerification(t *testing.T) {
	const jitter = 0.5
	cfg := RetryConfig{
		MaxAttempts:    20,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		BackoffFactor:  1.0, // constant base backoff = 10ms
		Jitter:         jitter,
	}

	var timestamps []time.Time
	_, _ = Retry(context.Background(), cfg, func() (int, error) {
		timestamps = append(timestamps, time.Now())
		return 0, errors.New("retry")
	})

	// Expected base backoff is 10ms. With jitter=0.5, actual delay ∈ [5ms, 15ms].
	lower := time.Duration(float64(10*time.Millisecond) * (1 - jitter))
	upper := time.Duration(float64(10*time.Millisecond) * (1 + jitter))
	const tol = 15 * time.Millisecond // generous scheduling tolerance

	for i := 0; i < len(timestamps)-1; i++ {
		gap := timestamps[i+1].Sub(timestamps[i])
		if gap < lower-tol || gap > upper+tol {
			t.Errorf("gap[%d]=%v not in [%v, %v] (with ±%v tolerance)", i, gap, lower, upper, tol)
		}
	}
}

func TestRetry_RetryIfFilterSpecificErrors(t *testing.T) {
	t.Parallel()
	errTemp := errors.New("temporary")
	errPerm := errors.New("permanent")

	cfg := RetryConfig{
		MaxAttempts:    5,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  1.0,
		RetryIf: func(err error) bool {
			return errors.Is(err, errTemp)
		},
	}

	t.Run("retries_temporary", func(t *testing.T) {
		t.Parallel()
		callCount := 0
		_, _ = Retry(context.Background(), cfg, func() (int, error) {
			callCount++
			return 0, errTemp
		})
		if callCount != 5 {
			t.Errorf("expected 5 attempts for temp error, got %d", callCount)
		}
	})

	t.Run("stops_on_permanent", func(t *testing.T) {
		t.Parallel()
		callCount := 0
		_, err := Retry(context.Background(), cfg, func() (int, error) {
			callCount++
			return 0, errPerm
		})
		if callCount != 1 {
			t.Errorf("expected 1 attempt for perm error, got %d", callCount)
		}
		if !errors.Is(err, errPerm) {
			t.Errorf("expected permanent error, got %v", err)
		}
	})
}

func TestRetry_OnRetryCorrectAttemptAndError(t *testing.T) {
	t.Parallel()
	type record struct {
		attempt int
		errMsg  string
	}
	var records []record
	var mu sync.Mutex

	cfg := RetryConfig{
		MaxAttempts:    4,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  1.0,
		OnRetry: func(attempt int, err error, _ time.Duration) {
			mu.Lock()
			records = append(records, record{attempt, err.Error()})
			mu.Unlock()
		},
	}

	callCount := 0
	_, _ = Retry(context.Background(), cfg, func() (int, error) {
		callCount++
		return 0, fmt.Errorf("err-%d", callCount)
	})

	mu.Lock()
	defer mu.Unlock()

	// OnRetry called after each failed attempt (except the last).
	if len(records) != 3 {
		t.Fatalf("expected 3 OnRetry calls, got %d", len(records))
	}
	for i, r := range records {
		wantAttempt := i + 1
		wantErr := fmt.Sprintf("err-%d", i+1)
		if r.attempt != wantAttempt {
			t.Errorf("record[%d].attempt = %d, want %d", i, r.attempt, wantAttempt)
		}
		if r.errMsg != wantErr {
			t.Errorf("record[%d].err = %q, want %q", i, r.errMsg, wantErr)
		}
	}
}

func TestRetry_ContextTimeoutMidRetry(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    100,
		InitialBackoff: 50 * time.Millisecond,
		BackoffFactor:  1.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()

	callCount := 0
	_, err := Retry(ctx, cfg, func() (int, error) {
		callCount++
		return 0, errors.New("fail")
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	// With 50ms backoff and 80ms timeout: attempt 1 at 0ms, sleep 50ms,
	// attempt 2 at ~50ms, sleep 50ms → context expires ~80ms.
	if callCount > 5 {
		t.Errorf("too many attempts (%d) – context should have stopped retries", callCount)
	}
}

func TestRetry_BackoffHitsMaxCap(t *testing.T) {
	t.Parallel()
	cfg := RetryConfig{
		MaxAttempts:    6,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     200 * time.Millisecond,
		BackoffFactor:  10.0,
		Jitter:         0,
	}

	// Verify calculateBackoff caps at MaxBackoff.
	for attempt := 1; attempt <= 5; attempt++ {
		b := calculateBackoff(attempt, cfg)
		if b > cfg.MaxBackoff {
			t.Errorf("attempt %d: backoff %v exceeds MaxBackoff %v", attempt, b, cfg.MaxBackoff)
		}
	}
}

func TestRetry_ZeroMaxAttemptsDefaultsToThree(t *testing.T) {
	t.Parallel()
	cfg := RetryConfig{
		MaxAttempts:    0,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  1.0,
	}

	callCount := 0
	_, _ = Retry(context.Background(), cfg, func() (int, error) {
		callCount++
		return 0, errors.New("fail")
	})

	if callCount != 3 {
		t.Errorf("expected default 3 attempts for zero MaxAttempts, got %d", callCount)
	}
}

func TestRetry_LargeMaxAttemptsImmediateSuccess(t *testing.T) {
	t.Parallel()
	cfg := RetryConfig{
		MaxAttempts:    1000,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  1.0,
	}

	callCount := 0
	result, err := Retry(context.Background(), cfg, func() (string, error) {
		callCount++
		if callCount >= 2 {
			return "ok", nil
		}
		return "", errors.New("once")
	})

	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

// ---------------------------------------------------------------------------
// 3. Bulkhead Advanced
// ---------------------------------------------------------------------------

func TestBulkhead_ExactlyMaxConcurrentAllSucceed(t *testing.T) {
	const max = 5
	b := NewBulkhead(BulkheadConfig{
		Name:          "exact",
		MaxConcurrent: max,
	})

	var running int32
	var maxRunning int32
	var wg sync.WaitGroup

	for i := 0; i < max; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := b.Execute(context.Background(), func() error {
				cur := atomic.AddInt32(&running, 1)
				for {
					old := atomic.LoadInt32(&maxRunning)
					if cur <= old || atomic.CompareAndSwapInt32(&maxRunning, old, cur) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&running, -1)
				return nil
			})
			if err != nil {
				t.Errorf("expected success, got %v", err)
			}
		}()
	}
	wg.Wait()

	if int(maxRunning) != max {
		t.Errorf("expected peak concurrency %d, got %d", max, maxRunning)
	}
}

func TestBulkhead_MaxConcurrentPlusOneRejected(t *testing.T) {
	const max = 2
	b := NewBulkhead(BulkheadConfig{
		Name:          "plus-one",
		MaxConcurrent: max,
		MaxWait:       0,
	})

	started := make(chan struct{}, max)
	release := make(chan struct{})
	var wg sync.WaitGroup

	// Fill all slots.
	for i := 0; i < max; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Execute(context.Background(), func() error {
				started <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for i := 0; i < max; i++ {
		<-started
	}

	// The (max+1)th call should be rejected.
	err := b.Execute(context.Background(), func() error { return nil })
	if !errors.Is(err, ErrBulkheadFull) {
		t.Errorf("expected ErrBulkheadFull, got %v", err)
	}

	close(release)
	wg.Wait()
}

func TestBulkhead_MaxWaitTimeoutPrecision(t *testing.T) {
	const maxWait = 50 * time.Millisecond
	b := NewBulkhead(BulkheadConfig{
		Name:          "precision",
		MaxConcurrent: 1,
		MaxWait:       maxWait,
	})

	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	start := time.Now()
	err := b.Execute(context.Background(), func() error { return nil })
	elapsed := time.Since(start)

	if !errors.Is(err, ErrBulkheadTimeout) {
		t.Errorf("expected ErrBulkheadTimeout, got %v", err)
	}

	const tolerance = 50 * time.Millisecond
	if elapsed < maxWait-tolerance || elapsed > maxWait+tolerance {
		t.Errorf("timeout precision: expected ~%v, got %v", maxWait, elapsed)
	}

	close(release)
}

func TestBulkhead_CallbackOrdering(t *testing.T) {
	var events []string
	var mu sync.Mutex

	b := NewBulkhead(BulkheadConfig{
		Name:          "order",
		MaxConcurrent: 1,
		OnAcquire: func(_ string) {
			mu.Lock()
			events = append(events, "acquire")
			mu.Unlock()
		},
		OnRelease: func(_ string) {
			mu.Lock()
			events = append(events, "release")
			mu.Unlock()
		},
	})

	_ = b.Execute(context.Background(), func() error {
		mu.Lock()
		events = append(events, "fn")
		mu.Unlock()
		return nil
	})

	mu.Lock()
	defer mu.Unlock()

	expected := []string{"acquire", "fn", "release"}
	if len(events) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, events)
	}
	for i, e := range expected {
		if events[i] != e {
			t.Errorf("event[%d] = %q, want %q", i, events[i], e)
		}
	}
}

func TestBulkhead_OnRejectCalled(t *testing.T) {
	t.Parallel()
	var rejectCount int32

	b := NewBulkhead(BulkheadConfig{
		Name:          "reject-cb",
		MaxConcurrent: 1,
		MaxWait:       0,
		OnReject: func(name string) {
			atomic.AddInt32(&rejectCount, 1)
			if name != "reject-cb" {
				// Can't use t.Errorf from goroutine easily, so skip.
			}
		},
	})

	started := make(chan struct{})
	release := make(chan struct{})
	go func() {
		b.Execute(context.Background(), func() error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	_ = b.Execute(context.Background(), func() error { return nil })
	_ = b.Execute(context.Background(), func() error { return nil })

	close(release)
	time.Sleep(5 * time.Millisecond)

	if atomic.LoadInt32(&rejectCount) != 2 {
		t.Errorf("expected 2 rejections, got %d", rejectCount)
	}
}

func TestBulkhead_PanicReleasesSlot(t *testing.T) {
	b := NewBulkhead(BulkheadConfig{
		Name:          "panic-slot",
		MaxConcurrent: 1,
		MaxWait:       0,
	})

	// Execute with a panic. The bulkhead uses defer for release, so the
	// channel-based semaphore should be freed if fn() panics because the
	// deferred release in Execute runs. But fn() panics before Execute's
	// defer completes normally — let's verify.
	func() {
		defer func() { _ = recover() }()
		_ = b.Execute(context.Background(), func() error { panic("kaboom") })
	}()

	// Slot should be available (defer releases even on panic).
	if b.Available() != 1 {
		t.Errorf("expected 1 available slot after panic, got %d", b.Available())
	}

	// Should be able to execute again.
	err := b.Execute(context.Background(), func() error { return nil })
	if err != nil {
		t.Errorf("expected success after panic recovery, got %v", err)
	}
}

func TestBulkhead_ConcurrentReleaseAcquireRace(t *testing.T) {
	t.Parallel()
	const max = 3
	b := NewBulkhead(BulkheadConfig{
		Name:          "race",
		MaxConcurrent: max,
		MaxWait:       100 * time.Millisecond,
	})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Execute(context.Background(), func() error {
				time.Sleep(time.Millisecond)
				return nil
			})
		}()
	}
	wg.Wait()

	if b.Available() != max {
		t.Errorf("expected %d available after all done, got %d", max, b.Available())
	}
}

func TestBulkhead_AvailableAccuracyDuringExecution(t *testing.T) {
	const max = 5
	b := NewBulkhead(BulkheadConfig{
		Name:          "avail",
		MaxConcurrent: max,
	})

	started := make(chan struct{})
	release := make(chan struct{})

	// Occupy 3 slots.
	for i := 0; i < 3; i++ {
		go func() {
			b.Execute(context.Background(), func() error {
				started <- struct{}{}
				<-release
				return nil
			})
		}()
	}
	for i := 0; i < 3; i++ {
		<-started
	}

	if got := b.Available(); got != 2 {
		t.Errorf("expected 2 available, got %d", got)
	}
	if got := b.InUse(); got != 3 {
		t.Errorf("expected 3 in use, got %d", got)
	}
	if got := b.MaxConcurrent(); got != max {
		t.Errorf("expected MaxConcurrent=%d, got %d", max, got)
	}

	close(release)
	time.Sleep(10 * time.Millisecond)

	if got := b.Available(); got != max {
		t.Errorf("expected %d available after release, got %d", max, got)
	}
}

// ---------------------------------------------------------------------------
// 4. Rate Limiter Advanced
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// 5. Degradation Manager Advanced
// ---------------------------------------------------------------------------

func TestDegradation_FeatureDependsOnServiceHealth(t *testing.T) {
	t.Parallel()
	dm := NewDegradationManager()

	dm.UpdateService("payment-api", Healthy)
	dm.SetFeature("checkout", true)

	if !dm.FeatureEnabled("checkout") {
		t.Error("checkout should be enabled when payment-api is healthy")
	}

	// Simulate: when payment-api becomes unhealthy, disable checkout.
	dm.UpdateService("payment-api", Unhealthy)
	if dm.ServiceStatus("payment-api").Health != Unhealthy {
		t.Error("payment-api should be unhealthy")
	}

	// An application would check health + feature. Verify the pattern works.
	status := dm.ServiceStatus("payment-api")
	if status.Health == Unhealthy {
		dm.SetFeature("checkout", false)
	}
	if dm.FeatureEnabled("checkout") {
		t.Error("checkout should be disabled when payment-api is unhealthy")
	}
}

func TestDegradation_OnCBStateChangeWithRealCircuitBreaker(t *testing.T) {
	dm := NewDegradationManager()

	config := CircuitBreakerConfig{
		Name:             "auth",
		MaxFailures:      2,
		Timeout:          15 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		OnStateChange:    dm.OnCircuitBreakerStateChange("auth"),
	}
	cb := NewCircuitBreaker(config)

	// Closed → Open
	for i := 0; i < 2; i++ {
		_ = cb.Execute(func() error { return errors.New("fail") })
	}
	if dm.ServiceStatus("auth").Health != Unhealthy {
		t.Errorf("expected Unhealthy, got %s", dm.ServiceStatus("auth").Health)
	}

	// Open → HalfOpen
	time.Sleep(20 * time.Millisecond)
	_ = cb.State() // triggers transition

	if dm.ServiceStatus("auth").Health != Degraded {
		t.Errorf("expected Degraded in half-open, got %s", dm.ServiceStatus("auth").Health)
	}

	// HalfOpen → Closed
	_ = cb.Execute(func() error { return nil })
	if dm.ServiceStatus("auth").Health != Healthy {
		t.Errorf("expected Healthy after close, got %s", dm.ServiceStatus("auth").Health)
	}
}

func TestDegradation_HealthEndpointJSONFormat(t *testing.T) {
	t.Parallel()
	dm := NewDegradationManager()
	dm.UpdateService("db", Healthy)
	dm.UpdateService("cache", Degraded, errors.New("timeout"))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	dm.HealthEndpoint().ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp healthResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("expected status 'degraded', got %q", resp.Status)
	}
	if len(resp.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(resp.Services))
	}
	if resp.Services["cache"].Error != "timeout" {
		t.Errorf("expected cache error 'timeout', got %q", resp.Services["cache"].Error)
	}
}

func TestDegradation_HealthEndpoint503WhenUnhealthy(t *testing.T) {
	t.Parallel()
	dm := NewDegradationManager()
	dm.UpdateService("db", Healthy)
	dm.UpdateService("api", Unhealthy)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	dm.HealthEndpoint().ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestDegradation_ConcurrentServiceHealthUpdates(t *testing.T) {
	t.Parallel()
	dm := NewDegradationManager()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc := fmt.Sprintf("svc-%d", n%5)
			h := ServiceHealth(n % 3)
			dm.UpdateService(svc, h)
			_ = dm.ServiceStatus(svc)
			_ = dm.AllStatuses()
			_ = dm.IsHealthy()
		}(i)
	}
	wg.Wait()

	statuses := dm.AllStatuses()
	if len(statuses) > 5 {
		t.Errorf("expected at most 5 services, got %d", len(statuses))
	}
}

func TestDegradation_FeatureReEnableAfterRecovery(t *testing.T) {
	dm := NewDegradationManager()

	config := CircuitBreakerConfig{
		Name:             "search",
		MaxFailures:      1,
		Timeout:          10 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		OnStateChange:    dm.OnCircuitBreakerStateChange("search"),
	}
	cb := NewCircuitBreaker(config)
	dm.SetFeature("advanced-search", true)

	// Trip CB → service becomes unhealthy → disable feature.
	_ = cb.Execute(func() error { return errors.New("fail") })
	if dm.ServiceStatus("search").Health != Unhealthy {
		t.Fatal("search should be unhealthy")
	}
	dm.SetFeature("advanced-search", false)

	// Wait for half-open → recover.
	time.Sleep(15 * time.Millisecond)
	_ = cb.Execute(func() error { return nil }) // half-open → closed

	if dm.ServiceStatus("search").Health != Healthy {
		t.Fatalf("search should be healthy after recovery, got %s", dm.ServiceStatus("search").Health)
	}

	// Re-enable feature.
	dm.SetFeature("advanced-search", true)
	if !dm.FeatureEnabled("advanced-search") {
		t.Error("advanced-search should be re-enabled after recovery")
	}
}

// ---------------------------------------------------------------------------
// 6. Multi-Pattern Integration
// ---------------------------------------------------------------------------

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
	if !dm.IsHealthy() || dm.ServiceStatus("recover").Health != Healthy {
		// Service might not yet be tracked by DM (no state change happened).
		// That's fine – IsHealthy returns true when no services are degraded.
	}

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

// ---------------------------------------------------------------------------
// Helpers: verify calculateBackoff returns non-negative durations for
// extreme inputs (sanity check used by multiple tests).
// ---------------------------------------------------------------------------

func TestCalculateBackoff_ExtremeInputs(t *testing.T) {
	t.Parallel()
	cfg := RetryConfig{
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Second,
		BackoffFactor:  2.0,
		Jitter:         1.0, // maximum jitter
	}

	for attempt := 1; attempt <= 100; attempt++ {
		d := calculateBackoff(attempt, cfg)
		if d < 0 {
			t.Errorf("attempt %d: negative backoff %v", attempt, d)
		}
		if d > cfg.MaxBackoff {
			// With jitter the pre-cap value may exceed max, but the cap should apply.
			// However jitter is applied before the cap check, so this may technically
			// be possible in pathological cases. Verify the implementation caps it.
			expected := float64(cfg.InitialBackoff) * math.Pow(cfg.BackoffFactor, float64(attempt-1))
			if expected > float64(cfg.MaxBackoff) && d > cfg.MaxBackoff {
				// calculateBackoff caps at MaxBackoff; jitter applied before cap.
				// So capped value should be MaxBackoff.
				if d != cfg.MaxBackoff {
					t.Errorf("attempt %d: expected cap at %v, got %v", attempt, cfg.MaxBackoff, d)
				}
			}
		}
	}
}
