package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimitConfig_ApplyDefaults(t *testing.T) {
	c := &RateLimitConfig{}
	c.applyDefaults()
	if c.RequestsPerMinute != 60 || c.KeyFunc == nil ||
		c.CleanupInterval != 5*time.Minute || c.BucketTTL != 10*time.Minute {
		t.Errorf("defaults wrong: %+v", c)
	}

	c2 := &RateLimitConfig{LimitFunc: func(*gin.Context) (string, int) { return "", 1 }}
	c2.applyDefaults()
	if c2.KeyFunc != nil {
		t.Errorf("KeyFunc should not be set when LimitFunc present")
	}
}

func TestNewRateLimiter_StopAndAllow(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 2})
	defer rl.Stop()

	allowed, limit, remaining, _, _ := rl.Allow("k", 2)
	if !allowed || limit != 2 || remaining < 0 {
		t.Errorf("first call: allowed=%v limit=%d remaining=%d", allowed, limit, remaining)
	}
	// Drain bucket.
	for i := 0; i < 3; i++ {
		rl.Allow("k", 2)
	}
	allowed, _, _, retry, _ := rl.Allow("k", 2)
	if allowed {
		t.Errorf("should be rate-limited after burst")
	}
	if retry <= 0 {
		t.Errorf("retryAfter should be > 0, got %f", retry)
	}

	rl.Stop()
	allowed, _, _, _, _ = rl.Allow("after-stop", 1)
	if !allowed {
		t.Error("rate limiter should keep making decisions after cleanup shutdown")
	}
}

func TestRateLimit_Middleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 1, KeyFunc: func(*gin.Context) string { return "fixed" }})
	defer rl.Stop()
	r := gin.New()
	r.Use(RateLimit(rl))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("first request: got %d", w.Code)
	}
	if w.Header().Get("X-RateLimit-Limit") != "1" {
		t.Errorf("limit header: %v", w.Header())
	}

	// Second request — rate limited.
	req = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be limited: got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Errorf("Retry-After missing")
	}
}

func TestRateLimit_LimitFuncTakesPrecedence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rl := NewRateLimiter(RateLimitConfig{
		RequestsPerMinute: 100, // ignored
		LimitFunc:         func(*gin.Context) (string, int) { return "tier-A", 1 },
	})
	defer rl.Stop()
	r := gin.New()
	r.Use(RateLimit(rl))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-RateLimit-Limit") != "1" {
		t.Errorf("LimitFunc rpm should win: %v", w.Header())
	}
}

func TestUserBasedKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)

	// Falls back to IP when no user_id.
	if got := UserBasedKey(c); got == "" {
		t.Errorf("expected non-empty IP fallback")
	}

	// Returns user_id when set.
	c.Set("user_id", "u-42")
	if got := UserBasedKey(c); got != "u-42" {
		t.Errorf("got %q want u-42", got)
	}

	// Falls back when value is wrong type.
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	c2.Set("user_id", 123)
	if got := UserBasedKey(c2); got == "u-42" {
		t.Errorf("non-string user_id should fall back, got %q", got)
	}
}

func TestIPBasedKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	if got := IPBasedKey(c); got == "" {
		t.Errorf("IPBasedKey returned empty")
	}
}
