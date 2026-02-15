package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimitConfig configures the rate limiting middleware.
type RateLimitConfig struct {
	// RequestsPerMinute is the maximum number of requests allowed per minute per key.
	RequestsPerMinute int
	// KeyFunc extracts the rate limit key from a request. Defaults to client IP.
	KeyFunc func(*gin.Context) string
}

// RateLimit returns a Gin middleware that applies per-key sliding-window rate limiting.
func RateLimit(cfg RateLimitConfig) gin.HandlerFunc {
	if cfg.RequestsPerMinute <= 0 {
		cfg.RequestsPerMinute = 60
	}
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = IPBasedKey
	}

	rl := &rateLimiter{
		requests: make(map[string][]time.Time),
		limit:    cfg.RequestsPerMinute,
	}
	go rl.cleanup()

	return func(c *gin.Context) {
		key := cfg.KeyFunc(c)
		if !rl.allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded",
			})
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

type rateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
}

func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-time.Minute)

	valid := filterByTime(rl.requests[key], cutoff)
	if len(valid) >= rl.limit {
		rl.requests[key] = valid
		return false
	}
	rl.requests[key] = append(valid, now)
	return true
}

func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-time.Minute)
		for key, times := range rl.requests {
			valid := filterByTime(times, cutoff)
			if len(valid) == 0 {
				delete(rl.requests, key)
			} else {
				rl.requests[key] = valid
			}
		}
		rl.mu.Unlock()
	}
}

func filterByTime(times []time.Time, cutoff time.Time) []time.Time {
	var result []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			result = append(result, t)
		}
	}
	return result
}
