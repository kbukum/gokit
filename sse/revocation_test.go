package sse_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/sse/testutil"
)

func TestRevocationInterruptsBlockedWriteBeforeDeadline(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		bus := endpointBus(t)
		cfg := handlerConfig()
		cfg.WriteTimeout = time.Hour
		lifetime, revoke := context.WithCancel(t.Context())
		cfg.Authorize = func(*http.Request) (sse.Access, error) {
			return sse.Access{Principal: "p", Route: "r", Lifetime: lifetime}, nil
		}
		h, err := sse.NewHandler(bus, cfg)
		if err != nil {
			t.Fatal(err)
		}
		server, peer := net.Pipe()
		defer server.Close()
		defer peer.Close()
		w := testutil.NewResponseWriter()
		w.Peer = server
		done := make(chan struct{})
		go func() { defer close(done); h.ServeHTTP(w, httptest.NewRequest("GET", "/", http.NoBody)) }()
		synctest.Wait()
		start := time.Now()
		revoke()
		<-done
		if time.Since(start) != 0 {
			t.Fatal("revocation waited for the write timeout")
		}
		if stats := bus.Stats(); stats.ActiveStreams != 0 || stats.QueueBytes != 0 {
			t.Fatalf("revocation leaked resources: %+v", stats)
		}
	})
}
