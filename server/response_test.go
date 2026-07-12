package server_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/server"
)

func newRespCtx() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/resource", http.NoBody)
	return c, w
}

func TestRespondSuccessHelpers(t *testing.T) {
	tests := []struct {
		name       string
		call       func(*gin.Context)
		wantStatus int
		wantData   bool
	}{
		{"ok", func(c *gin.Context) { server.RespondOK(c, "x") }, http.StatusOK, true},
		{"created", func(c *gin.Context) { server.RespondCreated(c, "x") }, http.StatusCreated, true},
		{"accepted", func(c *gin.Context) { server.RespondAccepted(c, "x") }, http.StatusAccepted, true},
		{"ok_meta", func(c *gin.Context) {
			server.RespondOKWithMeta(c, "x", &server.Meta{Page: 1, Total: 3})
		}, http.StatusOK, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := newRespCtx()
			tt.call(c)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if tt.wantData {
				var resp server.DataResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if resp.Data != "x" {
					t.Fatalf("data = %v, want x", resp.Data)
				}
			}
		})
	}
}

func TestRespondNoContent(t *testing.T) {
	c, _ := newRespCtx()
	server.RespondNoContent(c)
	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", c.Writer.Status(), http.StatusNoContent)
	}
}

func TestRespondErrorHelpers(t *testing.T) {
	tests := []struct {
		name       string
		call       func(*gin.Context)
		wantStatus int
	}{
		{"invalid_input", func(c *gin.Context) { server.RespondInvalidInput(c, "bad") }, http.StatusUnprocessableEntity},
		{"not_found", func(c *gin.Context) { server.RespondNotFound(c, "widget") }, http.StatusNotFound},
		{"unauthorized", func(c *gin.Context) { server.RespondUnauthorized(c, "no") }, http.StatusUnauthorized},
		{"forbidden", func(c *gin.Context) { server.RespondForbidden(c, "no") }, http.StatusForbidden},
		{"internal", func(c *gin.Context) { server.RespondInternalError(c, errors.New("boom")) }, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := newRespCtx()
			tt.call(c)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("content-type = %q, want application/problem+json", ct)
			}
		})
	}
}

func TestRespondWithError_WrapsPlainError(t *testing.T) {
	c, w := newRespCtx()
	server.RespondWithError(c, errors.New("unexpected"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

func TestRespondWithError_UsesAppErrorStatus(t *testing.T) {
	c, w := newRespCtx()
	server.RespondWithError(c, apperrors.NotFound("widget", "42"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected problem detail body")
	}
}
