package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

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
	_, err := Auth(fakeTokenValidator{}, storeClaims, WithQueryTokenParam("token"))
	if err == nil || !strings.Contains(err.Error(), "WithQueryTokenAllowedPaths") {
		t.Errorf("got %v want config error", err)
	}
}

func TestAuth_NilValidator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if _, err := Auth(nil, storeClaims); err == nil || !strings.Contains(err.Error(), "TokenValidator") {
		t.Errorf("got %v want non-nil TokenValidator error", err)
	}
}

func TestOptionalAuth_NilValidator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if _, err := OptionalAuth(nil, storeClaims); err == nil || !strings.Contains(err.Error(), "TokenValidator") {
		t.Errorf("got %v want non-nil TokenValidator error", err)
	}
}

func TestAuth_SkipPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h, err := Auth(fakeTokenValidator{err: errors.New("nope")}, storeClaims, WithSkipPaths("/public"))
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
	h, _ := Auth(fakeTokenValidator{claims: "u"}, storeClaims)
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
	h, _ := Auth(fakeTokenValidator{claims: "claims"}, storeClaims)
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
	h, _ := Auth(fakeTokenValidator{err: errors.New("bad")}, storeClaims)
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
		fakeTokenValidator{claims: "c"}, storeClaims,
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
	h, _ := Auth(fakeTokenValidator{claims: "c"}, storeClaims, WithScheme(""))
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

func TestAuth_NilClaimsSetter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if _, err := Auth(fakeTokenValidator{}, nil); err == nil || !strings.Contains(err.Error(), "ClaimsSetter") {
		t.Errorf("got %v want non-nil ClaimsSetter error", err)
	}
	if _, err := OptionalAuth(fakeTokenValidator{}, nil); err == nil || !strings.Contains(err.Error(), "ClaimsSetter") {
		t.Errorf("got %v want non-nil ClaimsSetter error", err)
	}
}

func FuzzExtractToken(f *testing.F) {
	// Build the seed header at runtime so no credential-like literal is
	// committed (avoids secret scanning) while still exercising scheme parsing.
	bearerHeader := "Bearer " + strings.Repeat("a", 24)
	f.Add(bearerHeader, "/x?token=def", "Authorization", "Bearer", "token")
	f.Add("", "/x?access=abc", "Authorization", "Bearer", "access")

	f.Fuzz(func(t *testing.T, header, rawURL, headerName, scheme, queryParam string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// httptest.NewRequest panics on various malformed inputs; skip those.
		var req *http.Request
		func() {
			defer func() { recover() }() //nolint:errcheck // intentional: skip panics from invalid fuzz inputs
			req = httptest.NewRequest(http.MethodGet, rawURL, http.NoBody)
		}()
		if req == nil {
			t.Skip("invalid request")
		}

		if headerName == "" {
			headerName = "Authorization"
		}
		req.Header.Set(headerName, header)
		c.Request = req

		_, _ = extractToken(c, &authOptions{
			headerName:              headerName,
			scheme:                  scheme,
			queryTokenParam:         queryParam,
			queryTokenAllowedPaths:  []string{c.Request.URL.Path},
			queryTokenWarningLogger: func(*gin.Context, string) {},
		})
	})
}
