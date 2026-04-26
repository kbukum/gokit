package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func TestStartupGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		markReady  bool
		opts       []StartupGateOption
		wantStatus int
	}{
		{
			name:       "not ready blocks API path",
			path:       "/api/v1/users",
			markReady:  false,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "not ready allows /health",
			path:       "/health",
			markReady:  false,
			wantStatus: http.StatusOK,
		},
		{
			name:       "not ready allows /ready",
			path:       "/ready",
			markReady:  false,
			wantStatus: http.StatusOK,
		},
		{
			name:       "not ready allows /alive",
			path:       "/alive",
			markReady:  false,
			wantStatus: http.StatusOK,
		},
		{
			name:       "not ready allows /metrics",
			path:       "/metrics",
			markReady:  false,
			wantStatus: http.StatusOK,
		},
		{
			name:       "ready allows API path",
			path:       "/api/v1/users",
			markReady:  true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "custom skip path allowed when not ready",
			path:       "/custom-probe",
			markReady:  false,
			opts:       []StartupGateOption{WithSkipStartupPaths("/custom-probe")},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gate := NewStartupGate(tt.opts...)
			if tt.markReady {
				gate.MarkReady()
			}

			w := httptest.NewRecorder()
			c, r := gin.CreateTestContext(w)
			r.Use(gate.Middleware())
			r.GET(tt.path, func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			c.Request = httptest.NewRequest(http.MethodGet, tt.path, http.NoBody)
			r.ServeHTTP(w, c.Request)

			if w.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestStartupGate_IsReady(t *testing.T) {
	t.Parallel()

	gate := NewStartupGate()
	if gate.IsReady() {
		t.Error("expected gate to not be ready initially")
	}

	gate.MarkReady()
	if !gate.IsReady() {
		t.Error("expected gate to be ready after MarkReady")
	}
}
