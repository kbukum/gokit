package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kbukum/gokit/server/httpx"
)

func FuzzParseBoolQuery(f *testing.F) {
	f.Add("true")
	f.Add("FALSE")
	f.Add("notabool")
	f.Fuzz(func(t *testing.T, raw string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/?flag="+raw, nil)
		_ = httpx.BoolQuery(c, "flag", false)
	})
}

func FuzzBindJSON(f *testing.F) {
	f.Add(`{"name":"a","age":1}`)
	f.Add(`{"name":123}`)
	f.Fuzz(func(t *testing.T, body string) {
		type req struct {
			Name string `json:"name" binding:"required"`
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		_, _ = httpx.BindJSON[req](c)
	})
}
