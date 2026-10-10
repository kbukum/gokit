package resilience

import (
	"sync"
	"time"

	"github.com/kbukum/gokit/util"
)

// DefaultMaxKeys bounds a keyed limiter whose config sets no MaxKeys.
const DefaultMaxKeys = 100_000

// saturatedScanInterval is how often a full limiter rescans for expired buckets before denying a new key, so a flood
// of new keys cannot force a full scan per call.
const saturatedScanInterval = time.Second

// KeyedRateLimiterConfig configures a keyed token-bucket limiter.
type KeyedRateLimiterConfig struct {
	// CleanupInterval is how often idle buckets are dropped (default 5m).
	CleanupInterval time.Duration
	// BucketTTL is how long an untouched bucket lives (default 10m).
	BucketTTL time.Duration
	// MaxKeys bounds how many keys hold a bucket at once (default [DefaultMaxKeys]). When full, the limiter drops
	// expired buckets and otherwise denies new keys with Saturated set, so callers fail closed instead of growing
	// without bound; keys that already hold a bucket keep their own limit.
	MaxKeys int
	// Clock supplies time (default util.SystemClock).
	Clock util.Clock
}

// RateLimitDecision captures the outcome of a rate-limit check.
type RateLimitDecision struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
	ResetAt    time.Time
	// Saturated reports a denial because the limiter holds MaxKeys buckets, not because this key ran out.
	Saturated bool
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
	lastScan    time.Time
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
	if cfg.MaxKeys <= 0 {
		cfg.MaxKeys = DefaultMaxKeys
	}
	if util.IsNil(cfg.Clock) {
		cfg.Clock = util.SystemClock{}
	}

	now := cfg.Clock.Now()
	rl := &KeyedRateLimiter{
		cfg:         cfg,
		nowFunc:     cfg.Clock.Now,
		lastCleanup: now,
		lastScan:    now,
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
	if !ok && !rl.admitKeyLocked(now) {
		return RateLimitDecision{Limit: normalizedLimit, RetryAfter: saturatedScanInterval, ResetAt: now.Add(saturatedScanInterval), Saturated: true}
	}
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
	rl.dropExpiredLocked(now)
	rl.lastCleanup = now
}

// admitKeyLocked reports whether a new key may get a bucket. A full limiter rescans for expired buckets at most once
// per saturatedScanInterval.
func (rl *KeyedRateLimiter) admitKeyLocked(now time.Time) bool {
	if len(rl.buckets) < rl.cfg.MaxKeys {
		return true
	}
	if now.Sub(rl.lastScan) < saturatedScanInterval {
		return false
	}
	rl.lastScan = now
	rl.dropExpiredLocked(now)
	return len(rl.buckets) < rl.cfg.MaxKeys
}

func (rl *KeyedRateLimiter) dropExpiredLocked(now time.Time) {
	for key, bucket := range rl.buckets {
		if now.Sub(bucket.lastAccess) > rl.cfg.BucketTTL {
			delete(rl.buckets, key)
		}
	}
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
