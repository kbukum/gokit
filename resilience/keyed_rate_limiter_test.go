package resilience

import (
	"testing"
	"time"
)

func TestKeyedRateLimiter_NormalizesInvalidLimitAndInterval(t *testing.T) {
	rl := NewKeyedRateLimiter(KeyedRateLimiterConfig{})
	defer rl.Stop()

	// limit <= 0 and interval <= 0 both hit the normalization branches.
	decision := rl.Allow("k", 0, 0)
	if decision.Limit != 1 {
		t.Fatalf("expected normalized limit 1, got %d", decision.Limit)
	}
	if !decision.Allowed {
		t.Fatal("expected first request to be allowed")
	}
}

func TestKeyedRateLimiter_RefillCapsAtMax(t *testing.T) {
	base := time.Unix(0, 0)
	now := base
	rl := NewKeyedRateLimiter(KeyedRateLimiterConfig{})
	rl.nowFunc = func() time.Time { return now }
	defer rl.Stop()

	// Consume one token, then advance far enough that refill would exceed the
	// bucket max, exercising the minFloat cap branch.
	if d := rl.Allow("k", 5, time.Second); !d.Allowed {
		t.Fatal("expected first allow")
	}
	now = base.Add(time.Hour)
	d := rl.Allow("k", 5, time.Second)
	if !d.Allowed {
		t.Fatal("expected allow after refill")
	}
	if d.Remaining != 4 {
		t.Fatalf("expected remaining capped to 4, got %d", d.Remaining)
	}
}

func TestKeyedRateLimiter_DeniesWhenExhausted(t *testing.T) {
	now := time.Unix(0, 0)
	rl := NewKeyedRateLimiter(KeyedRateLimiterConfig{})
	rl.nowFunc = func() time.Time { return now }
	defer rl.Stop()

	if d := rl.Allow("k", 1, time.Minute); !d.Allowed {
		t.Fatal("expected first allow")
	}
	d := rl.Allow("k", 1, time.Minute)
	if d.Allowed {
		t.Fatal("expected second request to be denied")
	}
	if d.RetryAfter <= 0 {
		t.Fatalf("expected positive RetryAfter, got %v", d.RetryAfter)
	}
}
