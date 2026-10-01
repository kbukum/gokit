package resilience

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestPolicyBudgetBoundsRateAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		policy := NewPolicy().WithTimeout(10 * time.Millisecond).
			WithRateLimiter(RateLimiterConfig{Rate: 1, Burst: 1})
		calls := 0
		invoke := func(context.Context) (int, error) { calls++; return 1, nil }
		if _, err := Execute(context.Background(), policy, invoke); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		_, err := Execute(context.Background(), policy, invoke)
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 || time.Since(start) != 10*time.Millisecond {
			t.Fatalf("admission escaped budget: calls=%d elapsed=%s error=%v", calls, time.Since(start), err)
		}
	})
}

func TestPolicyBudgetBoundsBulkheadAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		policy := NewPolicy().WithTimeout(10 * time.Millisecond).
			WithBulkhead(BulkheadConfig{MaxConcurrent: 1, MaxWait: time.Hour})
		policy.init()
		if err := policy.bh.acquire(context.Background()); err != nil {
			t.Fatal(err)
		}
		defer policy.bh.release()
		start := time.Now()
		_, err := Execute(context.Background(), policy, func(context.Context) (int, error) {
			t.Fatal("full bulkhead admitted work")
			return 0, nil
		})
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 10*time.Millisecond {
			t.Fatalf("admission escaped budget: elapsed=%s error=%v", time.Since(start), err)
		}
	})
}

func TestPolicyAdmissionTimeoutModes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     TimeoutMode
		existing time.Duration
		want     time.Duration
	}{
		{"override longer", TimeoutOverrideExisting, time.Second, 10 * time.Millisecond},
		{"preserve shorter", TimeoutOverrideExisting, time.Millisecond, time.Millisecond},
		{"if unset preserves", TimeoutIfUnset, time.Second, time.Second},
		{"if unset applies", TimeoutIfUnset, 0, 10 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				policy := NewPolicy().WithTimeout(10 * time.Millisecond).
					WithRateLimiter(RateLimiterConfig{Rate: 0.1, Burst: 1})
				policy.timeoutMode = tc.mode
				policy.init()
				if !policy.rl.Allow() {
					t.Fatal("initial token unavailable")
				}
				ctx := context.Background()
				if tc.existing > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, tc.existing)
					defer cancel()
				}
				start := time.Now()
				_, err := Execute(ctx, policy, func(context.Context) (int, error) {
					t.Fatal("admission outlived deadline")
					return 0, nil
				})
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != tc.want {
					t.Fatalf("elapsed=%s error=%v; want deadline after %s", time.Since(start), err, tc.want)
				}
			})
		})
	}
}
