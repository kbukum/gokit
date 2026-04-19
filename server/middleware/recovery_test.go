package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/server/middleware"
)

func TestRecovery_PanicReturnsCanonicalError(t *testing.T) {
	t.Parallel()

	handler := middleware.Recovery(nil)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	var pd apperrors.ProblemDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pd))
	assert.Equal(t, apperrors.ErrCodeInternal, pd.Code)
	assert.NotEmpty(t, pd.Detail)
	assert.False(t, pd.Retryable)
	assert.Equal(t, "/panic", pd.Instance)
}

func TestRecovery_NoPanicPassesThrough(t *testing.T) {
	t.Parallel()

	handler := middleware.Recovery(nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
