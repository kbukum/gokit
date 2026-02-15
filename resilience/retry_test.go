package resilience

import (
	"context"
	"errors"
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
