package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
)

func FuzzExtractToken(f *testing.F) {
	f.Add("Bearer abc", "/x?token=def", "Authorization", "Bearer", "token")
	f.Add("", "/x?access=abc", "Authorization", "Bearer", "access")

	f.Fuzz(func(t *testing.T, header, rawURL, headerName, scheme, queryParam string) {
		// httptest.NewRequest panics on invalid URLs; skip inputs that would panic.
		if _, err := url.ParseRequestURI(rawURL); err != nil {
			t.Skip("invalid URL")
		}

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodGet, rawURL, nil)
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
