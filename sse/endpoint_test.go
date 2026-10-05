package sse_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/sse"
	"github.com/kbukum/gokit/sse/testutil"
	"github.com/kbukum/gokit/util"
)

func endpointBus(t *testing.T) *sse.Bus {
	t.Helper()
	b, err := sse.NewBus(sse.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func handlerConfig() sse.HandlerConfig {
	cfg := sse.DefaultHandlerConfig()
	cfg.Authorize = sse.PublicAccess("public", "public")
	cfg.Logger = logging.NewDefault("sse-test")
	cfg.WriteTimeout = time.Second
	return cfg
}

func TestEndpointRejectsBeforeAllocation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		auth   sse.Authorizer
		cursor string
		status int
	}{
		{"unauthorized", func(*http.Request) (sse.Access, error) { return sse.Access{}, apperrors.Unauthorized("") }, "", 401},
		{"forbidden", func(*http.Request) (sse.Access, error) { return sse.Access{}, apperrors.Forbidden("") }, "", 403},
		{"unknown failure", func(*http.Request) (sse.Access, error) { return sse.Access{}, errors.New("secret") }, "", 500},
		{"invalid cursor", sse.PublicAccess("public", "public"), "bad", 422},
		{"invalid scope", sse.PublicAccess("public", "*"), "", 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := endpointBus(t)
			cfg := handlerConfig()
			cfg.Authorize = tc.auth
			h, err := sse.NewHandler(b, cfg)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest("GET", "/", http.NoBody)
			r.Header.Set("Last-Event-ID", tc.cursor)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || b.Stats().AllocatedQueues != 0 || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("status=%d body=%s stats=%+v", w.Code, w.Body, b.Stats())
			}
		})
	}
}

func TestEndpointStalledPeerDeadline(t *testing.T) {
	t.Parallel()
	b := endpointBus(t)
	cfg := handlerConfig()
	cfg.WriteTimeout = 50 * time.Millisecond
	h, err := sse.NewHandler(b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	server, peer := net.Pipe()
	defer server.Close()
	defer peer.Close()
	w := testutil.NewResponseWriter()
	w.Peer = server
	done := make(chan struct{})
	start := time.Now()
	go func() {
		defer close(done)
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", http.NoBody))
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("unread peer exceeded 2-second settling budget")
	}
	if elapsed := time.Since(start); elapsed < cfg.WriteTimeout || elapsed >= 2*time.Second {
		t.Fatalf("stalled write ended outside budget: %s", elapsed)
	}
	if stats := b.Stats(); stats.ActiveStreams != 0 || stats.QueueBytes != 0 {
		t.Fatalf("stalled peer leaked: %+v", stats)
	}
}

func TestEndpointMethodConfigDeadlineAndExpiry(t *testing.T) {
	t.Parallel()
	b := endpointBus(t)
	cfg := handlerConfig()
	if _, err := sse.NewHandler(nil, cfg); err == nil {
		t.Fatal("nil bus accepted")
	}
	if _, err := sse.NewHandler(b, sse.HandlerConfig{}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
	h, err := sse.NewHandler(b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	method := httptest.NewRecorder()
	h.ServeHTTP(method, httptest.NewRequest("POST", "/", http.NoBody))
	if method.Code != 405 || method.Header().Get("Allow") != "GET" {
		t.Fatal("method admitted")
	}
	unsupported := httptest.NewRecorder()
	h.ServeHTTP(unsupported, httptest.NewRequest("GET", "/", http.NoBody))
	if unsupported.Code != 500 || b.Stats().ActiveStreams != 0 {
		t.Fatal("unsupported deadline admitted")
	}
	expired, cancel := context.WithCancel(t.Context())
	cancel()
	cfg.Authorize = func(*http.Request) (sse.Access, error) {
		return sse.Access{Principal: "p", Route: "r", Lifetime: expired}, nil
	}
	h, err = sse.NewHandler(b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", http.NoBody))
	if w.Code != 401 {
		t.Fatal("expired lifetime admitted")
	}
}

func TestEndpointOverloadAndRetryFailure(t *testing.T) {
	t.Parallel()
	limits := sse.DefaultLimits()
	limits.MaxConnections, limits.MaxPerPrincipal = 1, 1
	b, err := sse.NewBus(limits)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	held, err := b.Subscribe(t.Context(), sse.SubscribeRequest{Principal: "public", Route: "public"})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	h, err := sse.NewHandler(b, handlerConfig())
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", http.NoBody))
	if w.Code != 503 || w.Header().Get("Retry-After") != "1" || b.Stats().AllocatedQueues != 1 || !strings.Contains(w.Body.String(), `"retryable":true`) {
		t.Fatalf("overload: %d %s %+v", w.Code, w.Body, b.Stats())
	}
	cfg := handlerConfig()
	cfg.Authorize = func(*http.Request) (sse.Access, error) {
		return sse.Access{}, apperrors.RateLimited().WithRetryAfter(1500 * time.Millisecond)
	}
	h, err = sse.NewHandler(b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", http.NoBody))
	if w.Header().Get("Retry-After") != "2" || !strings.Contains(w.Body.String(), `"retryAfter":1.5`) {
		t.Fatalf("retry delay: %s", w.Body)
	}
}

func TestEndpointLifetimeInterruptsStream(t *testing.T) {
	t.Parallel()
	b := endpointBus(t)
	lifetime, revoke := context.WithCancel(t.Context())
	cfg := handlerConfig()
	cfg.Authorize = func(*http.Request) (sse.Access, error) {
		return sse.Access{Principal: "alice", Route: "alice", Lifetime: lifetime}, nil
	}
	h, err := sse.NewHandler(b, cfg)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		h.ServeHTTP(w, r)
	}))
	defer server.Close()
	req, err := http.NewRequestWithContext(t.Context(), "GET", server.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if _, err := sse.NewDecoder(resp.Body).Next(); err != nil {
		t.Fatal(err)
	}
	revoke()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("revoked stream did not settle")
	}
	if b.Stats().ActiveStreams != 0 {
		t.Fatal("revocation leaked admission")
	}
}

func TestEndpointClockAndHeartbeat(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		b := endpointBus(t)
		cfg := handlerConfig()
		cfg.Clock = util.NewFakeClock(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC))
		h, err := sse.NewHandler(b, cfg)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		w := testutil.NewResponseWriter()
		done := make(chan struct{})
		go func() {
			defer close(done)
			h.ServeHTTP(w, httptest.NewRequestWithContext(ctx, "GET", "/", http.NoBody))
		}()
		synctest.Wait()
		time.Sleep(cfg.Heartbeat)
		synctest.Wait()
		cancel()
		<-done
		if !strings.Contains(w.Body.String(), ": keepalive\n\n") || w.Deadlines.Load() < 2 {
			t.Fatalf("missing renewed heartbeat: %s, deadlines=%d", w.Body, w.Deadlines.Load())
		}
		if b.Stats().ActiveStreams != 0 {
			t.Fatal("subscription leaked")
		}
	})
}

func TestEndpointWriteAndFlushFailureRelease(t *testing.T) {
	t.Parallel()
	for _, flush := range []bool{false, true} {
		b := endpointBus(t)
		h, err := sse.NewHandler(b, handlerConfig())
		if err != nil {
			t.Fatal(err)
		}
		w := testutil.NewResponseWriter()
		if flush {
			w.FlushErrorValue = io.ErrClosedPipe
		} else {
			w.WriteError = io.ErrClosedPipe
		}
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", http.NoBody))
		if b.Stats().ActiveStreams != 0 {
			t.Fatal("write failure leaked subscription")
		}
	}
}
