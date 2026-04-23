package httpx_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/server/httpx"
)

func TestParsePathUUID(t *testing.T) {
	t.Parallel()

	validID := uuid.New()

	tests := []struct {
		name    string
		param   string
		wantErr bool
		wantID  uuid.UUID
	}{
		{
			name:   "valid UUID",
			param:  validID.String(),
			wantID: validID,
		},
		{
			name:    "invalid UUID",
			param:   "not-a-uuid",
			wantErr: true,
		},
		{
			name:    "empty string",
			param:   "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/items/"+tc.param, nil)
			c.Params = gin.Params{{Key: "id", Value: tc.param}}

			got, err := httpx.ParsePathUUID(c, "id")

			if tc.wantErr {
				require.Error(t, err)
				var appErr *apperrors.AppError
				require.True(t, errors.As(err, &appErr))
				assert.Equal(t, apperrors.ErrCodeInvalidInput, appErr.Code)
				assert.Equal(t, uuid.Nil, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantID, got)
			}
		})
	}
}

func TestParsePathInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		param   string
		wantErr bool
		wantVal int
	}{
		{
			name:    "valid integer",
			param:   "42",
			wantVal: 42,
		},
		{
			name:    "negative integer",
			param:   "-7",
			wantVal: -7,
		},
		{
			name:    "not a number",
			param:   "abc",
			wantErr: true,
		},
		{
			name:    "empty string",
			param:   "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/items/"+tc.param, nil)
			c.Params = gin.Params{{Key: "id", Value: tc.param}}

			got, err := httpx.ParsePathInt(c, "id")

			if tc.wantErr {
				require.Error(t, err)
				var appErr *apperrors.AppError
				require.True(t, errors.As(err, &appErr))
				assert.Equal(t, 0, got)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantVal, got)
			}
		})
	}
}

func TestIntQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		key  string
		def  int
		want int
	}{
		{name: "present", url: "/?limit=50", key: "limit", def: 10, want: 50},
		{name: "missing uses default", url: "/", key: "limit", def: 10, want: 10},
		{name: "invalid uses default", url: "/?limit=abc", key: "limit", def: 10, want: 10},
		{name: "zero value", url: "/?limit=0", key: "limit", def: 10, want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tc.url, nil)

			got := httpx.IntQuery(c, tc.key, tc.def)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestStringQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		key  string
		def  string
		want string
	}{
		{name: "present", url: "/?sort=name", key: "sort", def: "id", want: "name"},
		{name: "missing uses default", url: "/", key: "sort", def: "id", want: "id"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tc.url, nil)

			got := httpx.StringQuery(c, tc.key, tc.def)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBoolQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		key  string
		def  bool
		want bool
	}{
		{name: "true", url: "/?active=true", key: "active", def: false, want: true},
		{name: "false", url: "/?active=false", key: "active", def: true, want: false},
		{name: "1 as true", url: "/?active=1", key: "active", def: false, want: true},
		{name: "missing uses default", url: "/", key: "active", def: true, want: true},
		{name: "invalid uses default", url: "/?active=maybe", key: "active", def: false, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tc.url, nil)

			got := httpx.BoolQuery(c, tc.key, tc.def)
			assert.Equal(t, tc.want, got)
		})
	}
}
