package resilience

import (
	"sync"
	"time"
)

// KeyedRateLimiterConfig configures a keyed token-bucket limiter.
type KeyedRateLimiterConfig struct {
	CleanupInterval time.Duration
	BucketTTL       time.Duration
}

// RateLimitDecision captures the outcome of a rate-limit check.
type RateLimitDecision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
}

type keyedBucket struct {
	limit      int
	interval   time.Duration
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
	lastAccess time.Time
}

// KeyedRateLimiter manages per-key token buckets.
type KeyedRateLimiter struct {
	cfg         KeyedRateLimiterConfig
	nowFunc     func() time.Time
	mu          sync.Mutex
	lastCleanup time.Time
	buckets     map[string]*keyedBucket
	stopCh      chan struct{}
	stoppedCh   chan struct{}
	stopOnce    sync.Once
}

// NewKeyedRateLimiter creates a keyed rate limiter.
func NewKeyedRateLimiter(cfg KeyedRateLimiterConfig) *KeyedRateLimiter {
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}
	if cfg.BucketTTL <= 0 {
		cfg.BucketTTL = 10 * time.Minute
	}

	now := time.Now()
	rl := &KeyedRateLimiter{
		cfg:         cfg,
		nowFunc:     time.Now,
		lastCleanup: now,
		buckets:     make(map[string]*keyedBucket),
		stopCh:      make(chan struct{}),
		stoppedCh:   make(chan struct{}),
	}
	go rl.runCleanup()
	return rl
}

// Allow applies a token-bucket limit for the given key and interval.
func (rl *KeyedRateLimiter) Allow(key string, limit int, interval time.Duration) RateLimitDecision {
	now := rl.nowFunc()
	normalizedLimit, normalizedInterval := normalizeKeyedLimit(limit, interval)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.cleanupLocked(now)

	bucket, ok := rl.buckets[key]
	if !ok || bucket.limit != normalizedLimit || bucket.interval != normalizedInterval {
		bucket = newKeyedBucket(normalizedLimit, normalizedInterval, now)
		rl.buckets[key] = bucket
	}

	decision := bucket.allow(now)
	decision.Limit = normalizedLimit
	return decision
}

// Stop releases the background cleanup loop. Existing buckets remain usable for direct Allow calls.
// It is safe to call multiple times.
func (rl *KeyedRateLimiter) Stop() {
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
		<-rl.stoppedCh
	})
}

func normalizeKeyedLimit(limit int, interval time.Duration) (int, time.Duration) {
	if limit <= 0 {
		limit = 1
	}
	if interval <= 0 {
		interval = time.Minute
	}
	return limit, interval
}

func (rl *KeyedRateLimiter) runCleanup() {
	ticker := time.NewTicker(rl.cfg.CleanupInterval)
	defer func() {
		ticker.Stop()
		close(rl.stoppedCh)
	}()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			rl.cleanupLocked(rl.nowFunc())
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *KeyedRateLimiter) cleanupLocked(now time.Time) {
	if now.Sub(rl.lastCleanup) < rl.cfg.CleanupInterval {
		return
	}
	for key, bucket := range rl.buckets {
		if now.Sub(bucket.lastAccess) > rl.cfg.BucketTTL {
			delete(rl.buckets, key)
		}
	}
	rl.lastCleanup = now
}

func newKeyedBucket(limit int, interval time.Duration, now time.Time) *keyedBucket {
	maxTokens := float64(limit)
	return &keyedBucket{
		limit:      limit,
		interval:   interval,
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: maxTokens / interval.Seconds(),
		lastRefill: now,
		lastAccess: now,
	}
}

func (b *keyedBucket) allow(now time.Time) RateLimitDecision {
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = minFloat(b.maxTokens, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now
	b.lastAccess = now

	if b.tokens >= 1 {
		b.tokens--
		remaining := int(b.tokens)
		return RateLimitDecision{
			Allowed:    true,
			Remaining:  remaining,
			RetryAfter: 0,
			ResetAt:    now.Add(b.resetAfter(remaining)),
		}
	}

	retryAfter := time.Duration(((1 - b.tokens) / b.refillRate) * float64(time.Second))
	return RateLimitDecision{
		Allowed:    false,
		Remaining:  0,
		RetryAfter: retryAfter,
		ResetAt:    now.Add(b.resetAfter(0)),
	}
}

func (b *keyedBucket) resetAfter(remaining int) time.Duration {
	used := b.limit - remaining
	if used <= 0 {
		return 0
	}
	return time.Duration((float64(used) / b.refillRate) * float64(time.Second))
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
