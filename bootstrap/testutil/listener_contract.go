package testutil

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/bootstrap"
)

// AssertListenerRoutes checks the route and fallback contract on a fresh listener without starting it. handler must dispatch through that listener's production routing stack.
func AssertListenerRoutes(t testing.TB, listener bootstrap.Listener, handler http.Handler) {
	t.Helper()
	answer := func(status int) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(status) })
	}
	check := func(path string, want int) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody))
		if rec.Code != want {
			t.Fatalf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
	check("/missing", http.StatusNotFound)
	for _, pattern := range []string{"", "/", "GET /{"} {
		if err := listener.Handle(pattern, answer(http.StatusOK)); err == nil {
			t.Fatalf("Handle(%q) accepted an invalid or reserved pattern", pattern)
		}
	}
	for _, nilHandler := range []http.Handler{nil, http.HandlerFunc(nil)} {
		if err := listener.Handle("/nil", nilHandler); err == nil {
			t.Fatal("Handle accepted a nil handler")
		}
		if err := listener.Fallback(nilHandler); err == nil {
			t.Fatal("Fallback accepted a nil handler")
		}
	}
	if err := listener.Handle("GET /mounted", answer(http.StatusNoContent)); err != nil {
		t.Fatal(err)
	}
	if err := listener.Handle("GET /mounted", answer(http.StatusOK)); err == nil {
		t.Fatal("Handle accepted a duplicate pattern")
	}
	for _, status := range []int{http.StatusAccepted, http.StatusTeapot} {
		if err := listener.Fallback(answer(status)); err != nil {
			t.Fatal(err)
		}
		check("/missing", status)
		check("/", status)
		check("/mounted", http.StatusNoContent)
	}
}
