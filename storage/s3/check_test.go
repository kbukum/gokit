package s3

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBucketCheckRefusesUnavailableOrUnauthorizedStorage(t *testing.T) {
	t.Parallel()
	for _, code := range []int{http.StatusOK, http.StatusNotFound, http.StatusForbidden, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodHead || r.URL.Path != "/bucket" {
					t.Error("readiness did not probe configured bucket")
				}
				w.WriteHeader(code)
			}))
			defer server.Close()
			s := newTestStorage(t, server.URL)
			if err := s.Check(t.Context()); (err == nil) != (code == http.StatusOK) {
				t.Fatalf("bucket check: %v", err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if err := s.Check(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("probe lost cancellation: %v", err)
			}
		})
	}
}
