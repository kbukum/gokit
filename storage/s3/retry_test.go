package s3

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestUploadDoesNotRedispatchAfterAmbiguousFailure(t *testing.T) {
	t.Parallel()
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	s := newTestStorage(t, srv.URL)
	if err := s.Upload(t.Context(), "owned-object", strings.NewReader("content")); err == nil {
		t.Fatal("ambiguous upload failure was hidden")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("upload was redispatched %d times", got)
	}
}
