package s3_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kbukum/gokit/storage/s3"
)

func TestStorageNeverFollowsRedirects(t *testing.T) {
	t.Parallel()
	var reached atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/other" {
			reached.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Location", "/other")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	backend, err := s3.NewStorage(t.Context(), &s3.Config{
		Bucket: "objects", Region: "us-east-1", Endpoint: server.URL, ForcePathStyle: true,
		AccessKeyID: "fixture", SecretAccessKey: "fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.Upload(t.Context(), "key", bytes.NewReader([]byte("value"))); err == nil {
		t.Fatal("redirected write appeared successful")
	}
	if reached.Load() != 0 {
		t.Fatal("storage followed a redirect")
	}
}
