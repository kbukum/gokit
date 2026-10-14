package s3

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kbukum/gokit/storage"
)

func TestCreateSendsAConditionalPutOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		status int
		exists bool
	}{
		{http.StatusOK, false},
		{http.StatusPreconditionFailed, true},
		{http.StatusConflict, false},
		{http.StatusServiceUnavailable, false},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodPut || r.Header.Get("If-None-Match") != "*" {
					t.Errorf("create was not a conditional PUT: %s If-None-Match=%q", r.Method, r.Header.Get("If-None-Match"))
				}
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			s := newTestStorage(t, srv.URL)
			err := s.Create(t.Context(), "owned-object", strings.NewReader("content"))
			if got := requests.Load(); got != 1 {
				t.Fatalf("create dispatched %d requests", got)
			}
			if errors.Is(err, storage.ErrExists) != tc.exists {
				t.Fatalf("occupied-key identity = %v, want %v: %v", errors.Is(err, storage.ErrExists), tc.exists, err)
			}
			if (tc.status == http.StatusOK) != (err == nil) {
				t.Fatalf("status %d returned %v", tc.status, err)
			}
		})
	}
}
