package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server"
	"github.com/kbukum/gokit/server/middleware"
)

func TestProblemWritersRejectUnencodableDetails(t *testing.T) {
	for name, write := range map[string]func(*gin.Context, error){
		"gin": server.RespondWithError,
		"http": func(c *gin.Context, err error) {
			middleware.WriteProblemDetails(c.Writer, c.Request, err)
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, response := newRespCtx()
			write(c, apperrors.InvalidInput("field", "invalid").WithDetails(map[string]any{"value": func() {}}))
			var got apperrors.ProblemDetail
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatalf("invalid problem JSON: %v", err)
			}
			if response.Code != http.StatusInternalServerError || got.Code != apperrors.ErrCodeInternal {
				t.Fatalf("response = %d %+v", response.Code, got)
			}
		})
	}
}

func TestResponseUsesSharedNormalization(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code apperrors.ErrorCode
	}{
		{context.Canceled, apperrors.ErrCodeCanceled},
		{context.DeadlineExceeded, apperrors.ErrCodeTimeout},
		{apperrors.Conflict("committed").WithCause(context.DeadlineExceeded), apperrors.ErrCodeConflict},
	} {
		c, response := newRespCtx()
		server.RespondWithError(c, tc.err)
		var got apperrors.ProblemDetail
		if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Code != tc.code || response.Code != apperrors.HTTPStatusFor(tc.code) {
			t.Fatalf("inconsistent normalization: %+v", got)
		}
	}
}

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

func TestRespondWithError_RetryAfterHeaderAndVocabulary(t *testing.T) {
	c, w := newRespCtx()
	appErr := apperrors.RateLimited().
		WithRetryAfter(1500 * time.Millisecond).
		WithReason("QUOTA_EXCEEDED").
		WithTraceID("trace-xyz")

	server.RespondWithError(c, appErr)

	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want %q (ceil of 1.5s)", got, "2")
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want problem+json", ct)
	}

	var pd apperrors.ProblemDetail
	if err := json.Unmarshal(w.Body.Bytes(), &pd); err != nil {
		t.Fatalf("decode problem detail: %v", err)
	}
	if pd.Instance != "/resource" {
		t.Errorf("instance = %q, want %q", pd.Instance, "/resource")
	}
	if pd.Reason != "QUOTA_EXCEEDED" {
		t.Errorf("reason = %q, want %q", pd.Reason, "QUOTA_EXCEEDED")
	}
	if pd.TraceID != "trace-xyz" {
		t.Errorf("traceId = %q, want %q", pd.TraceID, "trace-xyz")
	}
	if !pd.Retryable {
		t.Error("retryable should be true")
	}
	if pd.RetryAfterSeconds != 1.5 {
		t.Errorf("retryAfter = %v, want 1.5", pd.RetryAfterSeconds)
	}
}

func TestRespondWithError_NoRetryAfterHeaderWhenNotRetryable(t *testing.T) {
	c, w := newRespCtx()
	server.RespondWithError(c, apperrors.NotFound("widget", "42"))
	if got := w.Header().Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After = %q, want empty for a non-retryable error", got)
	}
}

func TestRespondWithError_LogsServerErrorViaContextLogger(t *testing.T) {
	var buf bytes.Buffer
	log, err := logging.New(
		&logging.Config{Level: "error", Format: "json", Timestamp: false},
		"server-test",
		logging.WithWriter(&buf),
	)
	if err != nil {
		t.Fatalf("build logger: %v", err)
	}

	c, w := newRespCtx()
	c.Request = c.Request.WithContext(logging.ContextWithLogger(c.Request.Context(), log))

	server.RespondWithError(c, errors.New("boom"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(buf.String(), "HTTP error response") {
		t.Fatalf("expected server-side 5xx log, got %q", buf.String())
	}
}

func TestRespondWithError_ClientErrorDoesNotLog(t *testing.T) {
	var buf bytes.Buffer
	log, err := logging.New(
		&logging.Config{Level: "error", Format: "json", Timestamp: false},
		"server-test",
		logging.WithWriter(&buf),
	)
	if err != nil {
		t.Fatalf("build logger: %v", err)
	}

	c, w := newRespCtx()
	c.Request = c.Request.WithContext(logging.ContextWithLogger(c.Request.Context(), log))

	server.RespondWithError(c, apperrors.NotFound("widget", "42"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no server-side log for a 4xx, got %q", buf.String())
	}
}
