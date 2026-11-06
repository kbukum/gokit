package fetchtest_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/modelhub/internal/fetchtest"
)

func TestAdapterGrantsOnlyListedOrigins(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	t.Cleanup(srv.Close)
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	t.Cleanup(other.Close)
	hc := fetchtest.Adapter(t, nil, srv.URL)
	if _, err := hc.Do(t.Context(), httpclient.Request{Method: http.MethodGet, Path: srv.URL}); err != nil {
		t.Fatalf("granted origin: %v", err)
	}
	if _, err := hc.Do(t.Context(), httpclient.Request{Method: http.MethodGet, Path: other.URL}); err == nil {
		t.Fatal("an origin outside the policy was reached")
	}
}
