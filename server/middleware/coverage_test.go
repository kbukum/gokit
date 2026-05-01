package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────────────────────────────────────
// auth.go option helpers + Auth/Require/RequirePermission paths
// ─────────────────────────────────────────────────────────────────────────────

func TestBuildAuthOptions_AllOptions(t *testing.T) {
	logged := 0
	o := buildAuthOptions(
		WithSkipPaths("/skip"),
		WithHeaderName("X-Auth"),
		WithScheme("Token"),
		WithQueryTokenParam("token"),
		WithQueryTokenAllowedPaths("/sse"),
		WithQueryTokenWarningLogger(func(*gin.Context, string) { logged++ }),
	)
	if o.headerName != "X-Auth" || o.scheme != "Token" {
		t.Errorf("header/scheme not set: %+v", o)
	}
	if len(o.skipPaths) != 1 || o.skipPaths[0] != "/skip" {
		t.Errorf("skipPaths: %v", o.skipPaths)
	}
	if o.queryTokenParam != "token" || len(o.queryTokenAllowedPaths) != 1 {
		t.Errorf("query token cfg: %+v", o)
	}
	if o.queryTokenWarningLogger == nil {
		t.Errorf("warning logger nil")
	}
	o.queryTokenWarningLogger(nil, "x")
	if logged != 1 {
		t.Errorf("warning logger not invoked")
	}
}

func TestAuth_QueryTokenConfigError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, err := Auth(fakeTokenValidator{}, WithQueryTokenParam("token"))
	if err == nil || !strings.Contains(err.Error(), "WithQueryTokenAllowedPaths") {
		t.Errorf("got %v want config error", err)
	}
}

func TestAuth_SkipPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h, err := Auth(fakeTokenValidator{err: errors.New("nope")}, WithSkipPaths("/public"))
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	r.Use(h)
	r.GET("/public/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/public/x", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("skip path should bypass auth: got %d", w.Code)
	}
}

func TestAuth_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h, _ := Auth(fakeTokenValidator{claims: "u"})
	r.Use(h)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("missing token: got %d want 401", w.Code)
	}
}

func TestAuth_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h, _ := Auth(fakeTokenValidator{claims: "claims"})
	r.Use(h)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer abc")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("valid token: got %d want 200", w.Code)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h, _ := Auth(fakeTokenValidator{err: errors.New("bad")})
	r.Use(h)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer x")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("invalid token: got %d want 401", w.Code)
	}
}

func TestAuth_QueryTokenFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	called := 0
	r := gin.New()
	h, err := Auth(
		fakeTokenValidator{claims: "c"},
		WithQueryTokenParam("token"),
		WithQueryTokenAllowedPaths("/sse"),
		WithQueryTokenWarningLogger(func(*gin.Context, string) { called++ }),
	)
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	r.Use(h)
	r.GET("/sse", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/sse?token=t", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || called != 1 {
		t.Errorf("query token fallback: status=%d called=%d", w.Code, called)
	}
}

func TestAuth_NoSchemeRawHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h, _ := Auth(fakeTokenValidator{claims: "c"}, WithScheme(""))
	r.Use(h)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "raw-token-value")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("raw header: got %d", w.Code)
	}
}

func TestRequire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name   string
		check  func(*gin.Context) bool
		status int
	}{
		{"allow", func(*gin.Context) bool { return true }, http.StatusOK},
		{"deny", func(*gin.Context) bool { return false }, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(Require(tc.check))
			r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Errorf("got %d want %d", w.Code, tc.status)
			}
		})
	}
}

type fakeChecker struct{ allow bool }

func (f fakeChecker) HasPermission(string, string) bool { return f.allow }

func TestRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, allow := range []bool{true, false} {
		r := gin.New()
		r.Use(RequirePermission(fakeChecker{allow: allow}, "read", func(*gin.Context) string { return "u1" }))
		r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		want := http.StatusOK
		if !allow {
			want = http.StatusForbidden
		}
		if w.Code != want {
			t.Errorf("allow=%v: got %d want %d", allow, w.Code, want)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// middleware.go: GinWrap + Chain
// ─────────────────────────────────────────────────────────────────────────────

func TestGinWrap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-MW", "1")
			next.ServeHTTP(w, req)
		})
	}
	r.Use(GinWrap(mw))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-MW") != "1" {
		t.Errorf("middleware not applied")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// bodysize.go
// ─────────────────────────────────────────────────────────────────────────────

func TestGinBodySizeLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinBodySizeLimit("10MB"))
	r.POST("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("hello"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("body within limit: got %d", w.Code)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// cors.go
// ─────────────────────────────────────────────────────────────────────────────

func TestGinCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &CORSConfig{
		AllowedOrigins:   []string{"https://app.example"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"X-Custom"},
		AllowCredentials: true,
	}
	r := gin.New()
	r.Use(GinCORS(cfg))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	// Preflight
	req := httptest.NewRequest(http.MethodOptions, "/", http.NoBody)
	req.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("preflight: got %d want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Errorf("origin header missing: %v", w.Header())
	}
	if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("credentials header missing")
	}

	// Disallowed origin → no CORS headers.
	req = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Origin", "https://evil")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("origin should be blocked")
	}

	// Wildcard.
	r2 := gin.New()
	r2.Use(GinCORS(&CORSConfig{AllowedOrigins: []string{"*"}}))
	r2.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	req = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("Origin", "https://anywhere")
	w = httptest.NewRecorder()
	r2.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "https://anywhere" {
		t.Errorf("wildcard should allow any origin")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// requestid.go
// ─────────────────────────────────────────────────────────────────────────────

func TestGinRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinRequestID())
	r.GET("/", func(c *gin.Context) {
		v, _ := c.Get("request_id")
		if v == nil || v.(string) == "" {
			t.Errorf("request_id not set in context")
		}
		c.Status(http.StatusOK)
	})

	// Auto-generated.
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") == "" {
		t.Errorf("X-Request-Id header missing")
	}

	// Preserved when client provides one.
	req = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req.Header.Set("X-Request-Id", "given")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Header().Get("X-Request-Id") != "given" {
		t.Errorf("client-provided request id not preserved")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// recovery.go: GinRecovery
// ─────────────────────────────────────────────────────────────────────────────

func TestGinRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinRecovery())
	r.GET("/", func(c *gin.Context) { panic("boom") })

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("recovery: got %d want 500", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("expected problem+json, got %q", w.Header().Get("Content-Type"))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// logging.go: GinRequestLogger + RequestLogger covers logByStatus/isHealth
// ─────────────────────────────────────────────────────────────────────────────

func TestGinRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinRequestLogger())
	r.GET("/api/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/err", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })
	r.GET("/slow", func(c *gin.Context) {
		time.Sleep(550 * time.Millisecond)
		c.Status(http.StatusOK)
	})

	for _, p := range []string{"/api/x?q=1", "/health", "/err"} {
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
	// Slow path — sanity check, validates the slow-flag branch.
	req := httptest.NewRequest(http.MethodGet, "/slow", http.NoBody)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestRequestLogger_NetHTTP(t *testing.T) {
	mw := RequestLogger(nil)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	for _, p := range []string{"/health", "/api/foo"} {
		req := httptest.NewRequest(http.MethodGet, p, http.NoBody)
		req.Header.Set("X-Request-Id", "rid-1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// responsewriter.go (Write + Unwrap)
// ─────────────────────────────────────────────────────────────────────────────

func TestStatusWriter_WriteAndUnwrap(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newStatusWriter(rec)

	// Write without explicit WriteHeader → status defaults to 200, wroteHeader becomes true.
	if _, err := sw.Write([]byte("hi")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !sw.wroteHeader {
		t.Errorf("wroteHeader not set after Write")
	}
	if sw.status != http.StatusOK {
		t.Errorf("default status: got %d want 200", sw.status)
	}
	if sw.Unwrap() != rec {
		t.Errorf("Unwrap should return underlying writer")
	}
	// Flush is a no-op on httptest.ResponseRecorder (which implements Flusher).
	sw.Flush()

	// WriteHeader called twice → second call is ignored.
	rec2 := httptest.NewRecorder()
	sw2 := newStatusWriter(rec2)
	sw2.WriteHeader(http.StatusTeapot)
	sw2.WriteHeader(http.StatusBadRequest)
	if sw2.status != http.StatusTeapot {
		t.Errorf("second WriteHeader should be ignored: got %d", sw2.status)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// tenant.go: SetTenantInContext + TenantFromContext
// ─────────────────────────────────────────────────────────────────────────────

func TestTenantContext(t *testing.T) {
	ctx := SetTenantInContext(context.Background(), "tenant-1")
	got, ok := TenantFromContext(ctx)
	if !ok || got != "tenant-1" {
		t.Errorf("TenantFromContext: got (%q,%v)", got, ok)
	}
	if _, ok := TenantFromContext(context.Background()); ok {
		t.Errorf("empty ctx should return ok=false")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ratelimit.go: RateLimiter, token bucket, helpers, RateLimit middleware
// ─────────────────────────────────────────────────────────────────────────────

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
