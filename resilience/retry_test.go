package resilience

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestRetry_SucceedsOnFirstAttempt(t *testing.T) {
	cfg := DefaultRetryConfig()
	callCount := 0

	result, err := Retry(context.Background(), cfg, func() (string, error) {
		callCount++
		return "success", nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %s", result)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestRetry_SucceedsAfterRetry(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  2.0,
	}
	callCount := 0

	result, err := Retry(context.Background(), cfg, func() (string, error) {
		callCount++
		if callCount < 3 {
			return "", errors.New("temporary error")
		}
		return "success", nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %s", result)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestRetry_ExceedsMaxAttempts(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  2.0,
	}
	callCount := 0
	testErr := errors.New("persistent error")

	_, err := Retry(context.Background(), cfg, func() (string, error) {
		callCount++
		return "", testErr
	})

	if !errors.Is(err, testErr) {
		t.Errorf("expected testErr, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestRetry_RespectsContext(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    10,
		InitialBackoff: 100 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	callCount := 0
	_, err := Retry(ctx, cfg, func() (string, error) {
		callCount++
		return "", errors.New("error")
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
	// Should have made at least 1 attempt but not all 10
	if callCount >= 10 {
		t.Errorf("expected fewer than 10 calls, got %d", callCount)
	}
}

func TestRetry_RetryIfFilter(t *testing.T) {
	retryableErr := errors.New("retryable")
	nonRetryableErr := errors.New("non-retryable")

	cfg := RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  2.0,
		RetryIf: func(err error) bool {
			return errors.Is(err, retryableErr)
		},
	}

	// Test with retryable error
	callCount := 0
	_, _ = Retry(context.Background(), cfg, func() (string, error) {
		callCount++
		return "", retryableErr
	})
	if callCount != 3 {
		t.Errorf("expected 3 calls for retryable error, got %d", callCount)
	}

	// Test with non-retryable error
	callCount = 0
	_, err := Retry(context.Background(), cfg, func() (string, error) {
		callCount++
		return "", nonRetryableErr
	})
	if callCount != 1 {
		t.Errorf("expected 1 call for non-retryable error, got %d", callCount)
	}
	if !errors.Is(err, nonRetryableErr) {
		t.Errorf("expected nonRetryableErr, got %v", err)
	}
}

func TestRetry_OnRetryCallback(t *testing.T) {
	var retries []int
	var mu sync.Mutex

	cfg := RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  2.0,
		OnRetry: func(attempt int, err error, backoff time.Duration) {
			mu.Lock()
			retries = append(retries, attempt)
			mu.Unlock()
		},
	}

	_, _ = Retry(context.Background(), cfg, func() (string, error) {
		return "", errors.New("error")
	})

	mu.Lock()
	defer mu.Unlock()

	// OnRetry called before each retry, not before first attempt
	if len(retries) != 2 {
		t.Errorf("expected 2 OnRetry calls, got %d", len(retries))
	}
	if retries[0] != 1 || retries[1] != 2 {
		t.Errorf("expected attempts [1, 2], got %v", retries)
	}
}

func TestRetryFunc(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts:    3,
		InitialBackoff: time.Millisecond,
		BackoffFactor:  2.0,
	}
	callCount := 0

	err := RetryFunc(context.Background(), cfg, func() error {
		callCount++
		if callCount < 2 {
			return errors.New("error")
		}
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestRetryWithBackoff(t *testing.T) {
	callCount := 0

	result, err := RetryWithBackoff(context.Background(), 3, func() (int, error) {
		callCount++
		if callCount < 2 {
			return 0, errors.New("error")
		}
		return 42, nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
}

func TestCalculateBackoff(t *testing.T) {
	cfg := RetryConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
		BackoffFactor:  2.0,
		Jitter:         0, // No jitter for predictable testing
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond}, // 100 * 2^0
		{2, 200 * time.Millisecond}, // 100 * 2^1
		{3, 400 * time.Millisecond}, // 100 * 2^2
		{4, 800 * time.Millisecond}, // 100 * 2^3
		{5, 1 * time.Second},        // Capped at max
		{6, 1 * time.Second},        // Still capped
	}

	for _, tt := range tests {
		got := calculateBackoff(tt.attempt, cfg)
		if got != tt.expected {
			t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, got)
		}
	}
}

func TestBackoffDelay(t *testing.T) {
	cfg := RetryConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     250 * time.Millisecond,
		BackoffFactor:  2.0,
		Jitter:         0,
	}

	if got := BackoffDelay(3, cfg); got != 250*time.Millisecond {
		t.Fatalf("BackoffDelay() = %v, want 250ms", got)
	}
}

// seededRand returns a deterministic Float64 source for reproducible jitter.
func seededRand(seed uint64) func() float64 {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)).Float64
}

func TestCalculateBackoff_InjectedRandIsDeterministic(t *testing.T) {
	t.Parallel()
	newCfg := func() RetryConfig {
		return RetryConfig{
			InitialBackoff: 100 * time.Millisecond,
			MaxBackoff:     10 * time.Second,
			BackoffFactor:  2.0,
			Jitter:         0.5,
			Rand:           seededRand(42),
		}
	}

	// Same seed → identical backoff sequence.
	a, b := newCfg(), newCfg()
	for attempt := 1; attempt <= 5; attempt++ {
		if got, want := calculateBackoff(attempt, a), calculateBackoff(attempt, b); got != want {
			t.Fatalf("attempt %d: %v != %v (injected RNG must be deterministic)", attempt, got, want)
		}
	}
}

func TestCalculateBackoff_InjectedRandControlsJitter(t *testing.T) {
	t.Parallel()
	// Rand fixed at 0.0 → jitter term is (0*2-1)*range = -range (lower bound).
	low := RetryConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		BackoffFactor:  1.0,
		Jitter:         0.5,
		Rand:           func() float64 { return 0.0 },
	}
	// Rand fixed at just below 1.0 → jitter term approaches +range (upper bound).
	high := low
	high.Rand = func() float64 { return 0.999999 }

	base := 100 * time.Millisecond
	gotLow := calculateBackoff(1, low)
	gotHigh := calculateBackoff(1, high)

	if gotLow >= base {
		t.Errorf("Rand=0 should push backoff below base: got %v, base %v", gotLow, base)
	}
	if gotHigh <= base {
		t.Errorf("Rand≈1 should push backoff above base: got %v, base %v", gotHigh, base)
	}
}

func TestBackoffDelay_ClampsAttemptBelowOne(t *testing.T) {
	cfg := RetryConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		BackoffFactor:  2.0,
		Strategy:       ExponentialBackoff,
	}
	if got, want := BackoffDelay(0, cfg), BackoffDelay(1, cfg); got != want {
		t.Fatalf("attempt<1 should clamp to 1: got %v, want %v", got, want)
	}
}

func TestCalculateBackoff_ConstantAndLinearStrategies(t *testing.T) {
	base := 100 * time.Millisecond
	constCfg := RetryConfig{InitialBackoff: base, MaxBackoff: time.Minute, Strategy: ConstantBackoff}
	if got := calculateBackoff(3, constCfg); got != base {
		t.Fatalf("constant backoff should stay at base: got %v", got)
	}

	linearCfg := RetryConfig{InitialBackoff: base, MaxBackoff: time.Minute, Strategy: LinearBackoff}
	if got := calculateBackoff(3, linearCfg); got != 3*base {
		t.Fatalf("linear backoff attempt 3 should be 3x base: got %v", got)
	}
}

func TestCalculateBackoff_NegativeJitterFallsBackToInitial(t *testing.T) {
	cfg := RetryConfig{
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		Strategy:       ConstantBackoff,
		Jitter:         2.0,                         // large jitter range
		Rand:           func() float64 { return 0 }, // pushes jitter fully negative
	}
	if got := calculateBackoff(1, cfg); got != cfg.InitialBackoff {
		t.Fatalf("negative backoff should fall back to InitialBackoff: got %v", got)
	}
}

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
	if runtime.GOOS == "windows" {
		// Test asserts 5–15 ms gaps with ±15 ms tolerance, but Windows'
		// timer granularity (~16 ms) snaps sleeps onto coarse ticks and
		// blows past the upper bound. Tracked in #114.
		t.Skip("Windows clock resolution coarser than test tolerances")
	}
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
