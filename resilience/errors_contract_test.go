package resilience

import (
	"context"
	"errors"
	"testing"

	apperr "github.com/kbukum/gokit/errors"
)

// Resilience runtime failures are typed AppErrors carrying a stable error code
// and HTTP status, yet the exported sentinels still match under errors.Is so
// existing callers keep working.

func TestRuntimeSentinelsAreTypedAppErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    error
		code   apperr.ErrorCode
		status int
	}{
		{"circuit_open", ErrCircuitOpen, apperr.ErrCodeServiceUnavailable, 503},
		{"bulkhead_full", ErrBulkheadFull, apperr.ErrCodeRateLimited, 429},
		{"bulkhead_timeout", ErrBulkheadTimeout, apperr.ErrCodeRateLimited, 429},
		{"rate_limited", ErrRateLimited, apperr.ErrCodeRateLimited, 429},
		{"max_retries", ErrMaxRetriesExceeded, apperr.ErrCodeServiceUnavailable, 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			appErr, ok := apperr.AsAppError(tc.err)
			if !ok {
				t.Fatalf("expected AppError, got %T", tc.err)
			}
			if appErr.Code != tc.code {
				t.Fatalf("code = %q, want %q", appErr.Code, tc.code)
			}
			if appErr.HTTPStatus != tc.status {
				t.Fatalf("status = %d, want %d", appErr.HTTPStatus, tc.status)
			}
		})
	}
}

func TestCircuitOpenSentinelMatchesUnderIs(t *testing.T) {
	t.Parallel()

	cb := NewCircuitBreaker(CircuitBreakerConfig{MaxFailures: 1, HalfOpenMaxCalls: 1})
	// Trip the breaker, then confirm the open error still matches the sentinel.
	_ = cb.Execute(func() error { return errors.New("boom") })
	err := cb.Execute(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected errors.Is match against ErrCircuitOpen, got %v", err)
	}
}

func TestRateLimitedSentinelMatchesUnderIs(t *testing.T) {
	t.Parallel()

	rl := NewRateLimiter(RateLimiterConfig{Rate: 1, Burst: 1})
	_ = rl.Execute(func() error { return nil }) // consume the single token
	err := rl.Execute(func() error { return nil })
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected errors.Is match against ErrRateLimited, got %v", err)
	}
}

func TestBulkheadFullSentinelMatchesUnderIs(t *testing.T) {
	t.Parallel()

	bh := NewBulkhead(BulkheadConfig{Name: "b", MaxConcurrent: 1})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_ = bh.Execute(context.Background(), func() error {
			close(done)
			<-release
			return nil
		})
	}()
	<-done
	err := bh.Execute(context.Background(), func() error { return nil })
	close(release)
	if !errors.Is(err, ErrBulkheadFull) {
		t.Fatalf("expected errors.Is match against ErrBulkheadFull, got %v", err)
	}
}

func TestRetryExhaustionReturnsMaxRetriesWithCause(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	_, err := Retry(context.Background(), RetryConfig{
		MaxAttempts: 2,
		RetryIf:     func(error) bool { return true },
	}, func() (struct{}, error) {
		return struct{}{}, boom
	})

	if !errors.Is(err, ErrMaxRetriesExceeded) {
		t.Fatalf("expected errors.Is match against ErrMaxRetriesExceeded, got %v", err)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("expected the underlying cause to be preserved, got %v", err)
	}
	appErr, ok := apperr.AsAppError(err)
	if !ok {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code != apperr.ErrCodeServiceUnavailable || appErr.HTTPStatus != 503 {
		t.Fatalf("code/status = %q/%d, want SERVICE_UNAVAILABLE/503", appErr.Code, appErr.HTTPStatus)
	}
}
