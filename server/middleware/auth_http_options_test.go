package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apperrors "github.com/kbukum/gokit/errors"
)

func TestHTTPAuthInjectedErrorWriter(t *testing.T) {
	t.Parallel()
	cause := errors.New("private diagnostic")
	want := apperrors.Unauthorized("").WithCause(cause)
	request := httptest.NewRequest(http.MethodPost, "/example.Service/Method", http.NoBody)
	request.Header.Add("Cookie", "session=one")
	request.Header.Add("Cookie", "session=two")
	called := false
	mw, err := HTTPAuth(requestAuthenticator(func(*http.Request) (string, error) {
		return "", want
	}), storeClaims, WithAuthErrorWriter(func(w http.ResponseWriter, r *http.Request, err error) {
		called = true
		if r != request || !errors.Is(err, want) || !errors.Is(err, cause) || len(r.Header.Values("Cookie")) != 2 {
			t.Fatal("error writer lost original request or error")
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("custom rejection writer did not receive no-store policy")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("failed authentication reached handler")
	})).ServeHTTP(w, request)
	if !called || w.Code != http.StatusUnauthorized || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" || w.Body.Len() != 0 {
		t.Fatalf("custom response overwritten: %d %s", w.Code, w.Body)
	}
}

func TestHTTPAuthRejectsNilErrorWriterOptions(t *testing.T) {
	t.Parallel()
	auth := requestAuthenticator(func(*http.Request) (string, error) { return "alice", nil })
	for _, option := range []HTTPAuthOption{nil, WithAuthErrorWriter(nil)} {
		if _, err := HTTPAuth(auth, storeClaims, option); err == nil {
			t.Fatal("accepted nil authentication error writer configuration")
		}
	}
}
