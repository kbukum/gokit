package testutil_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kbukum/gokit/testutil"
)

func TestHTTPDrainClosesLiveConnections(t *testing.T) {
	t.Parallel()
	for _, tls := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "http2"}[tls], func(t *testing.T) {
			t.Parallel()
			exited := make(chan struct{})
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(exited)
				w.Header().Set("Content-Type", "text/event-stream")
				if _, err := w.Write([]byte("data: ready\n\n")); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
			}))
			if tls {
				srv.EnableHTTP2 = true
				srv.StartTLS()
			} else {
				srv.Start()
			}
			t.Cleanup(srv.Close)
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if tls && resp.ProtoMajor != 2 {
				t.Fatalf("expected HTTP/2, got %s", resp.Proto)
			}
			drainCtx, drainCancel := context.WithTimeout(ctx, 50*time.Millisecond)
			defer drainCancel()
			start := time.Now()
			if err := testutil.CloseHTTPServer(drainCtx, srv); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("forced connection closure not observable: %v", err)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("drain exceeded settling ceiling")
			}
			select {
			case <-exited:
			case <-ctx.Done():
				t.Fatal("owned handler was not released")
			}
			if err := testutil.CloseHTTPServer(ctx, srv); err != nil {
				t.Fatalf("repeated close: %v", err)
			}
		})
	}
}
