package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestHTTPAuthProblemEncodingEscapesHTML(t *testing.T) {
	t.Parallel()
	mw, err := HTTPAuth(requestAuthenticator(func(*http.Request) (string, error) {
		return "", apperrors.Forbidden("<script>alert('untrusted')</script>")
	}), storeClaims)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("rejected identity reached handler")
	})).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", http.NoBody))
	if w.Header().Get("Content-Type") != "application/problem+json" || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), `\u003cscript\u003e`) {
		t.Fatalf("unsafe problem encoding: %s", w.Body)
	}
}

type authWriteFailure struct {
	*httptest.ResponseRecorder
	writes int
}

func (w *authWriteFailure) Write([]byte) (int, error) {
	w.writes++
	return 0, errors.New("disconnected")
}

func TestHTTPAuthFailedResponseWriteDoesNotRetry(t *testing.T) {
	t.Parallel()
	mw, err := HTTPAuth(requestAuthenticator(func(*http.Request) (string, error) {
		return "", apperrors.Unauthorized("")
	}), storeClaims)
	if err != nil {
		t.Fatal(err)
	}
	w := &authWriteFailure{ResponseRecorder: httptest.NewRecorder()}
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("rejected identity reached handler")
	})).ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", http.NoBody))
	if w.Code != http.StatusUnauthorized || w.writes != 1 {
		t.Fatalf("write retried: status=%d writes=%d", w.Code, w.writes)
	}
}

func TestWriteProblemDetailsNormalizesAndHintsRetry(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	WriteProblemDetails(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody), apperrors.ServiceUnavailable("peer").WithRetryAfter(1500*time.Millisecond).WithCause(errors.New("dial 10.0.0.1")))
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "2" || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "10.0.0.1") {
		t.Fatalf("problem = %d %v %s", w.Code, w.Header(), w.Body)
	}

	w = httptest.NewRecorder()
	WriteProblemDetails(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody), errors.New("private detail"))
	if w.Code != http.StatusInternalServerError || w.Header().Get("Content-Type") != "application/problem+json" || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("problem = %d %v %s", w.Code, w.Header(), w.Body)
	}

	w = httptest.NewRecorder()
	WriteProblemDetails(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody), nil)
	if w.Code != http.StatusInternalServerError || w.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("nil problem = %d %v %s", w.Code, w.Header(), w.Body)
	}
}

func TestWriteProblemDetailsFallsBackWhenDetailsCannotEncode(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	failure := apperrors.InvalidInput("field", "bad").WithDetails(map[string]any{"fn": func() {}})
	WriteProblemDetails(w, httptest.NewRequest(http.MethodGet, "/", http.NoBody), failure)
	if w.Code != http.StatusInternalServerError || w.Header().Get("Content-Type") != "application/problem+json" || w.Body.Len() == 0 {
		t.Fatalf("problem = %d %v %q", w.Code, w.Header(), w.Body)
	}
}
