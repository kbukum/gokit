package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExtractToken_QueryParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		header    string
		queryURL  string
		opts      *authOptions
		wantToken string
		wantOK    bool
	}{
		{
			name:      "token from Authorization header",
			header:    "Bearer header-token",
			queryURL:  "/test",
			opts:      &authOptions{headerName: "Authorization", scheme: "Bearer"},
			wantToken: "header-token",
			wantOK:    true,
		},
		{
			name:      "token from query param when header absent",
			header:    "",
			queryURL:  "/test?token=query-token",
			opts:      &authOptions{headerName: "Authorization", scheme: "Bearer", queryTokenParam: "token"},
			wantToken: "query-token",
			wantOK:    true,
		},
		{
			name:      "header takes precedence over query param",
			header:    "Bearer header-token",
			queryURL:  "/test?token=query-token",
			opts:      &authOptions{headerName: "Authorization", scheme: "Bearer", queryTokenParam: "token"},
			wantToken: "header-token",
			wantOK:    true,
		},
		{
			name:     "no token anywhere returns false",
			header:   "",
			queryURL: "/test",
			opts:     &authOptions{headerName: "Authorization", scheme: "Bearer", queryTokenParam: "token"},
			wantOK:   false,
		},
		{
			name:     "query param ignored when not configured",
			header:   "",
			queryURL: "/test?token=query-token",
			opts:     &authOptions{headerName: "Authorization", scheme: "Bearer"},
			wantOK:   false,
		},
		{
			name:      "raw header without scheme and query fallback",
			header:    "",
			queryURL:  "/test?access_token=raw-query",
			opts:      &authOptions{headerName: "X-API-Key", scheme: "", queryTokenParam: "access_token"},
			wantToken: "raw-query",
			wantOK:    true,
		},
		{
			name:      "raw header value returned when no scheme",
			header:    "raw-header-value",
			queryURL:  "/test",
			opts:      &authOptions{headerName: "X-API-Key", scheme: ""},
			wantToken: "raw-header-value",
			wantOK:    true,
		},
		{
			name:     "bad scheme falls through to query param",
			header:   "Basic wrongscheme",
			queryURL: "/test?token=fallback-token",
			opts:     &authOptions{headerName: "Authorization", scheme: "Bearer", queryTokenParam: "token"},
			wantToken: "fallback-token",
			wantOK:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tt.queryURL, nil)
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
		})
	}
}

func TestWithQueryTokenParam_Option(t *testing.T) {
	t.Parallel()

	o := &authOptions{}
	WithQueryTokenParam("access_token")(o)
	if o.queryTokenParam != "access_token" {
		t.Errorf("got queryTokenParam = %q, want %q", o.queryTokenParam, "access_token")
	}
}
