package httpx_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/server/httpx"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type createReq struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

func TestBindJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr bool
		errCode apperrors.ErrorCode
	}{
		{
			name:    "valid payload",
			body:    `{"name":"Alice","email":"alice@example.com"}`,
			wantErr: false,
		},
		{
			name:    "malformed JSON",
			body:    `{bad json`,
			wantErr: true,
			errCode: apperrors.ErrCodeInvalidInput,
		},
		{
			name:    "validation failure missing name",
			body:    `{"email":"alice@example.com"}`,
			wantErr: true,
			errCode: apperrors.ErrCodeInvalidInput,
		},
		{
			name:    "validation failure bad email",
			body:    `{"name":"Alice","email":"not-an-email"}`,
			wantErr: true,
			errCode: apperrors.ErrCodeInvalidInput,
		},
		{
			name:    "empty body",
			body:    ``,
			wantErr: true,
			errCode: apperrors.ErrCodeInvalidInput,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")

			got, err := httpx.BindJSON[createReq](c)

			if tc.wantErr {
				require.Error(t, err)
				var appErr *apperrors.AppError
				require.ErrorAs(t, err, &appErr)
				assert.Equal(t, tc.errCode, appErr.Code)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, "Alice", got.Name)
				assert.Equal(t, "alice@example.com", got.Email)
			}
		})
	}
}

type searchQuery struct {
	Q    string `form:"q" validate:"required"`
	Page int    `form:"page"`
}

func TestBindQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		query   string
		wantErr bool
		wantQ   string
	}{
		{
			name:  "valid query",
			query: "q=hello&page=2",
			wantQ: "hello",
		},
		{
			name:    "missing required field",
			query:   "page=1",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/?"+tc.query, http.NoBody)

			got, err := httpx.BindQuery[searchQuery](c)

			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tc.wantQ, got.Q)
			}
		})
	}
}

func FuzzParseBoolQuery(f *testing.F) {
	f.Add("true")
	f.Add("FALSE")
	f.Add("notabool")
	f.Fuzz(func(t *testing.T, raw string) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/?flag="+url.QueryEscape(raw), http.NoBody)
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
