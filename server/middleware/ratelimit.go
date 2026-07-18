package middleware

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/resilience"
)

// RateLimitConfig configures the rate limiting middleware.
type RateLimitConfig struct {
	// RequestsPerMinute is the default RPM when LimitFunc is not set.
	RequestsPerMinute int

	// KeyFunc extracts the rate limit key from a request. Defaults to client IP. Ignored when LimitFunc is set.
	KeyFunc func(*gin.Context) string

	// LimitFunc resolves both the bucket key and RPM for a request. When set, KeyFunc and RequestsPerMinute are ignored.
	LimitFunc func(*gin.Context) (key string, rpm int)

	// CleanupInterval controls how often stale buckets are evicted. Default: 5 minutes.
	CleanupInterval time.Duration

	// BucketTTL is how long an unused bucket survives before eviction. Default: 10 minutes.
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

// RateLimiterInstance adapts the shared resilience keyed limiter for HTTP.
type RateLimiterInstance struct {
	cfg     RateLimitConfig
	limiter *resilience.KeyedRateLimiter
}

// NewRateLimiter creates a rate limiter.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiterInstance {
	cfg.applyDefaults()
	return &RateLimiterInstance{
		cfg: cfg,
		limiter: resilience.NewKeyedRateLimiter(resilience.KeyedRateLimiterConfig{
			CleanupInterval: cfg.CleanupInterval,
			BucketTTL:       cfg.BucketTTL,
		}),
	}
}

// Stop releases background cleanup resources. Existing buckets remain usable so in-flight shutdown paths do not race with decision making.
func (rl *RateLimiterInstance) Stop() {
	if rl == nil || rl.limiter == nil {
		return
	}
	rl.limiter.Stop()
}

// Allow checks whether the given key is permitted another request at the given RPM. Returns (allowed, limit, remaining, retryAfterSecs, resetUnix).
func (rl *RateLimiterInstance) Allow(key string, rpm int) (allowed bool, limit, remaining int, retryAfterSecs float64, resetUnix int64) {
	decision := rl.limiter.Allow(key, rpm, time.Minute)
	return decision.Allowed, decision.Limit, decision.Remaining, decision.RetryAfter.Seconds(), decision.ResetAt.Unix()
}

// RateLimit returns a Gin middleware that applies per-key rate limiting with standard rate limit response headers.
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
