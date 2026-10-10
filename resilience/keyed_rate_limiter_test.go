package resilience

import (
	"testing"
	"time"

	"github.com/kbukum/gokit/util"
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

func TestKeyedRateLimiter_UsesInjectedClock(t *testing.T) {
	clock := util.NewFakeClock(time.Time{})
	rl := NewKeyedRateLimiter(KeyedRateLimiterConfig{Clock: clock})
	defer rl.Stop()

	if d := rl.Allow("k", 1, time.Minute); !d.Allowed {
		t.Fatal("expected first allow")
	}
	if d := rl.Allow("k", 1, time.Minute); d.Allowed {
		t.Fatal("expected denial before the clock advances")
	}
	clock.Advance(time.Minute)
	d := rl.Allow("k", 1, time.Minute)
	if !d.Allowed {
		t.Fatal("expected allow after the injected clock refills the bucket")
	}
	if !d.ResetAt.After(clock.Now()) {
		t.Fatalf("ResetAt %v must come from the injected clock %v", d.ResetAt, clock.Now())
	}
}

func TestKeyedRateLimiter_DefaultsBoundKeys(t *testing.T) {
	rl := NewKeyedRateLimiter(KeyedRateLimiterConfig{})
	defer rl.Stop()
	if rl.cfg.MaxKeys != DefaultMaxKeys {
		t.Fatalf("expected default MaxKeys %d, got %d", DefaultMaxKeys, rl.cfg.MaxKeys)
	}
}

func TestKeyedRateLimiter_SaturatedDeniesNewKeysOnly(t *testing.T) {
	clock := util.NewFakeClock(time.Time{})
	rl := NewKeyedRateLimiter(KeyedRateLimiterConfig{Clock: clock, MaxKeys: 2, BucketTTL: time.Minute, CleanupInterval: time.Hour})
	defer rl.Stop()

	for _, key := range []string{"a", "b"} {
		if d := rl.Allow(key, 5, time.Second); !d.Allowed || d.Saturated {
			t.Fatalf("expected %s to be admitted, got %+v", key, d)
		}
	}
	d := rl.Allow("c", 5, time.Second)
	if d.Allowed || !d.Saturated || d.RetryAfter <= 0 || d.Limit != 5 {
		t.Fatalf("expected a saturated denial for a new key, got %+v", d)
	}
	if d := rl.Allow("a", 5, time.Second); !d.Allowed || d.Saturated {
		t.Fatalf("expected an existing key to keep its own limit, got %+v", d)
	}
	if got := len(rl.buckets); got != 2 {
		t.Fatalf("expected the limiter to stay at 2 keys, got %d", got)
	}
}

func TestKeyedRateLimiter_SaturatedRescansForExpiredBuckets(t *testing.T) {
	clock := util.NewFakeClock(time.Time{})
	rl := NewKeyedRateLimiter(KeyedRateLimiterConfig{Clock: clock, MaxKeys: 1, BucketTTL: 2 * time.Second, CleanupInterval: time.Hour})
	defer rl.Stop()

	if d := rl.Allow("old", 1, time.Second); !d.Allowed {
		t.Fatal("expected first key to be admitted")
	}
	// Within the rescan interval a full limiter denies without scanning, even once the old bucket has expired.
	clock.Advance(500 * time.Millisecond)
	if d := rl.Allow("new", 1, time.Second); !d.Saturated {
		t.Fatalf("expected saturation while the old bucket is live, got %+v", d)
	}
	clock.Advance(3 * time.Second)
	if d := rl.Allow("new", 1, time.Second); !d.Allowed || d.Saturated {
		t.Fatalf("expected the rescan to drop the expired bucket and admit the new key, got %+v", d)
	}
	if _, ok := rl.buckets["old"]; ok {
		t.Fatal("expected the expired bucket to be dropped")
	}
	// A rescan that finds nothing expired still denies, and the next one waits for the interval.
	clock.Advance(1100 * time.Millisecond)
	if d := rl.Allow("other", 1, time.Second); !d.Saturated {
		t.Fatalf("expected saturation while the only bucket is live, got %+v", d)
	}
}
