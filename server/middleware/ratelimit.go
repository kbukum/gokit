package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitConfig configures the rate limiting middleware.
type RateLimitConfig struct {
	// RequestsPerMinute is the default RPM when LimitFunc is not set.
	RequestsPerMinute int

	// KeyFunc extracts the rate limit key from a request. Defaults to client IP.
	// Ignored when LimitFunc is set.
	KeyFunc func(*gin.Context) string

	// LimitFunc resolves both the bucket key and RPM for a request.
	// When set, KeyFunc and RequestsPerMinute are ignored.
	// This enables tiered rate limits (e.g. per-user-type).
	LimitFunc func(*gin.Context) (key string, rpm int)

	// CleanupInterval controls how often stale buckets are evicted.
	// Default: 5 minutes.
	CleanupInterval time.Duration

	// BucketTTL is how long an unused bucket survives before eviction.
	// Default: 10 minutes.
	BucketTTL time.Duration
}

func (cfg *RateLimitConfig) applyDefaults() {
	if cfg.RequestsPerMinute <= 0 {
		cfg.RequestsPerMinute = 60
	}
	if cfg.LimitFunc == nil && cfg.KeyFunc == nil {
		cfg.KeyFunc = IPBasedKey
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 5 * time.Minute
	}
	if cfg.BucketTTL <= 0 {
		cfg.BucketTTL = 10 * time.Minute
	}
}

// RateLimiterInstance manages per-key token buckets with background cleanup.
type RateLimiterInstance struct {
	cfg     RateLimitConfig
	buckets sync.Map // map[string]*tokenBucket
	stopCh  chan struct{}
	nowFunc func() time.Time // for testing
}

// NewRateLimiter creates a rate limiter and starts the background eviction goroutine.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiterInstance {
	cfg.applyDefaults()
	rl := &RateLimiterInstance{
		cfg:     cfg,
		stopCh:  make(chan struct{}),
		nowFunc: time.Now,
	}
	go rl.cleanup()
	return rl
}

// Stop terminates the background cleanup goroutine.
func (rl *RateLimiterInstance) Stop() {
	close(rl.stopCh)
}

// Allow checks whether the given key is permitted another request at the given RPM.
// Returns (allowed, limit, remaining, retryAfterSecs, resetUnix).
func (rl *RateLimiterInstance) Allow(key string, rpm int) (allowed bool, limit int, remaining int, retryAfterSecs float64, resetUnix int64) {
	now := rl.nowFunc()

	val, _ := rl.buckets.LoadOrStore(key, newTokenBucket(rpm, now))
	bucket := val.(*tokenBucket)

	allowed, remaining, retryAfter := bucket.allow(now)
	resetUnix = now.Add(time.Duration(float64(time.Second) * (float64(rpm-remaining) / (float64(rpm) / 60.0)))).Unix()

	return allowed, rpm, remaining, retryAfter, resetUnix
}

func (rl *RateLimiterInstance) cleanup() {
	ticker := time.NewTicker(rl.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-rl.stopCh:
			return
		case <-ticker.C:
			now := rl.nowFunc()
			rl.buckets.Range(func(key, value any) bool {
				b := value.(*tokenBucket)
				b.mu.Lock()
				stale := now.Sub(b.lastAccess) > rl.cfg.BucketTTL
				b.mu.Unlock()
				if stale {
					rl.buckets.Delete(key)
				}
				return true
			})
		}
	}
}

// RateLimit returns a Gin middleware that applies per-key token-bucket rate limiting
// with standard rate limit response headers.
func RateLimit(limiter *RateLimiterInstance) gin.HandlerFunc {
	return func(c *gin.Context) {
		key, rpm := resolveKeyAndRPM(c, limiter.cfg)

		allowed, limit, remaining, retryAfter, resetUnix := limiter.Allow(key, rpm)

		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Header("X-RateLimit-Reset", strconv.FormatInt(resetUnix, 10))

		if !allowed {
			c.Header("Retry-After", fmt.Sprintf("%.0f", math.Ceil(retryAfter)))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}

		c.Next()
	}
}

// IPBasedKey extracts the client IP for use as a rate limit key.
func IPBasedKey(c *gin.Context) string {
	return c.ClientIP()
}

// UserBasedKey extracts the user_id from the context, falling back to client IP.
func UserBasedKey(c *gin.Context) string {
	if uid, exists := c.Get("user_id"); exists {
		if s, ok := uid.(string); ok && s != "" {
			return s
		}
	}
	return c.ClientIP()
}

func resolveKeyAndRPM(c *gin.Context, cfg RateLimitConfig) (key string, rpm int) {
	if cfg.LimitFunc != nil {
		return cfg.LimitFunc(c)
	}
	return cfg.KeyFunc(c), cfg.RequestsPerMinute
}

// tokenBucket implements a classic token-bucket algorithm.
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64   // tokens per second
	lastRefill time.Time // last time tokens were refilled
	lastAccess time.Time // last time the bucket was used (for TTL eviction)
}

func newTokenBucket(rpm int, now time.Time) *tokenBucket {
	maxT := float64(rpm)
	return &tokenBucket{
		tokens:     maxT,
		maxTokens:  maxT,
		refillRate: maxT / 60.0,
		lastRefill: now,
		lastAccess: now,
	}
}

// allow consumes one token and returns (allowed, remaining tokens, seconds until next token).
func (b *tokenBucket) allow(now time.Time) (allowed bool, remaining int, retryAfterSecs float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = math.Min(b.maxTokens, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now
	b.lastAccess = now

	if b.tokens >= 1 {
		b.tokens--
		remaining := int(b.tokens)
		return true, remaining, 0
	}

	retryAfter := (1 - b.tokens) / b.refillRate
	return false, 0, retryAfter
}
