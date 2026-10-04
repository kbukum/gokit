package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
