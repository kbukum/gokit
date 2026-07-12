package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
	req := httptest.NewRequest(http.MethodGet, "/panic", http.NoBody)
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
	req := httptest.NewRequest(http.MethodGet, "/ok", http.NoBody)
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGinRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.GinRecovery())
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
