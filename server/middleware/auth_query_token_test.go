package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Subtests share a `warned` flag and a single httptest recorder factory, so
// they cannot run in parallel. The parent t.Parallel() is intentionally
// omitted to keep the test deterministic.
//
//nolint:tparallel // subtests share mutable state by design (warned flag)
func TestExtractToken_QueryParam(t *testing.T) {
	warned := false
	warn := func(_ *gin.Context, _ string) { warned = true }
	tests := []struct {
		name       string
		header     string
		queryURL   string
		opts       *authOptions
		wantToken  string
		wantOK     bool
		wantWarned bool
	}{
		{name: "token from Authorization header", header: "Bearer header-token", queryURL: "/test", opts: &authOptions{headerName: "Authorization", scheme: "Bearer"}, wantToken: "header-token", wantOK: true},
		{name: "token from query param when header absent and path allowed", queryURL: "/test?token=query-token", opts: &authOptions{headerName: "Authorization", scheme: "Bearer", queryTokenParam: "token", queryTokenAllowedPaths: []string{"/test"}, queryTokenWarningLogger: warn}, wantToken: "query-token", wantOK: true, wantWarned: true},
		{name: "query param denied on non-whitelisted path", queryURL: "/other?token=query-token", opts: &authOptions{headerName: "Authorization", scheme: "Bearer", queryTokenParam: "token", queryTokenAllowedPaths: []string{"/test"}, queryTokenWarningLogger: warn}, wantOK: false},
		{name: "header takes precedence over query param", header: "Bearer header-token", queryURL: "/test?token=query-token", opts: &authOptions{headerName: "Authorization", scheme: "Bearer", queryTokenParam: "token", queryTokenAllowedPaths: []string{"/test"}, queryTokenWarningLogger: warn}, wantToken: "header-token", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warned = false

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tt.queryURL, http.NoBody)
			if tt.header != "" {
				c.Request.Header.Set(tt.opts.headerName, tt.header)
			}

			token, ok := extractToken(c, tt.opts)
			if ok != tt.wantOK {
				t.Errorf("extractToken() ok = %v, want %v", ok, tt.wantOK)
			}
			if token != tt.wantToken {
				t.Errorf("extractToken() token = %q, want %q", token, tt.wantToken)
			}
			if warned != tt.wantWarned {
				t.Errorf("warning called = %v, want %v", warned, tt.wantWarned)
			}
		})
	}
}

func TestAuthOptions_QueryTokenValidation(t *testing.T) {
	t.Parallel()
	o := buildAuthOptions(WithQueryTokenParam("token"))
	if err := o.validateQueryTokenConfig(); err == nil {
		t.Fatal("expected validation error when missing whitelist and logger")
	}
}
