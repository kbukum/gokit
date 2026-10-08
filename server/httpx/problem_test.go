package httpx_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server/httpx"
)

func TestWriteProblemDetails(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		err   error
		code  apperrors.ErrorCode
		retry string
	}{
		{"nil", nil, apperrors.ErrCodeInternal, ""},
		{"unknown", errors.New("private cause"), apperrors.ErrCodeInternal, ""},
		{"invalid", apperrors.New("INVALID_CODE", "private cause"), apperrors.ErrCodeInternal, ""},
		{"unencodable", apperrors.InvalidInput("input", "bad").WithDetails(map[string]any{"value": func() {}}), apperrors.ErrCodeInternal, ""},
		{"client", apperrors.NotFound("item", ""), apperrors.ErrCodeNotFound, ""},
		{"fractional retry", apperrors.ServiceUnavailable("peer").WithRetryAfter(1500 * time.Millisecond), apperrors.ErrCodeServiceUnavailable, "2"},
		{"whole retry", apperrors.ServiceUnavailable("peer").WithRetryAfter(time.Second), apperrors.ErrCodeServiceUnavailable, "1"},
		{"no retry", apperrors.ServiceUnavailable("peer").WithRetryable(false), apperrors.ErrCodeServiceUnavailable, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			log, err := logging.New(&logging.Config{Level: "debug", Format: "json"}, "problem-test", logging.WithWriter(&logs))
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/resource", http.NoBody)
			req = req.WithContext(logging.ContextWithLogger(req.Context(), log))
			response := httptest.NewRecorder()
			response.Header().Set("Retry-After", "99")
			httpx.WriteProblemDetails(response, req, tc.err)
			var problem apperrors.ProblemDetail
			if err := json.Unmarshal(response.Body.Bytes(), &problem); err != nil {
				t.Fatal(err)
			}
			if response.Code != apperrors.HTTPStatusFor(tc.code) || problem.Code != tc.code || problem.Instance != "/resource" || strings.Contains(response.Body.String(), "private cause") {
				t.Fatalf("response = %d %s", response.Code, response.Body)
			}
			if response.Header().Get("Content-Type") != "application/problem+json" || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Retry-After") != tc.retry {
				t.Fatalf("headers = %v", response.Header())
			}
			if got, want := strings.Contains(logs.String(), "HTTP error response"), response.Code >= 500; got != want {
				t.Fatalf("5xx logging = %v, want %v", got, want)
			}
		})
	}
}

type failedProblemWriter struct{ *httptest.ResponseRecorder }

func (failedProblemWriter) Write([]byte) (int, error) { return 0, errors.New("connection closed") }

func TestWriteProblemDetailsLogsWriteFailure(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	log, err := logging.New(&logging.Config{Level: "warn", Format: "json"}, "problem-test", logging.WithWriter(&logs))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	req = req.WithContext(logging.ContextWithLogger(req.Context(), log))
	httpx.WriteProblemDetails(failedProblemWriter{httptest.NewRecorder()}, req, apperrors.Unauthorized(""))
	if !strings.Contains(logs.String(), "HTTP problem response write failed") || !strings.Contains(logs.String(), "connection closed") {
		t.Fatalf("missing write failure log: %s", &logs)
	}
}
