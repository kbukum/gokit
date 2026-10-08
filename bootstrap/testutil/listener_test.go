package testutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/kbukum/gokit/component"
)

func TestListenerLifecycle(t *testing.T) {
	t.Parallel()
	l := NewListener("public")
	l.Handle("GET /ok", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	if l.URL() != "" || l.Health(t.Context()).Status != component.StatusUnhealthy {
		t.Fatal("unstarted listener has a URL or reports healthy")
	}
	if err := l.Drain(t.Context()); err != nil {
		t.Fatalf("Drain before Start = %v", err)
	}
	if err := l.Stop(t.Context()); err != nil {
		t.Fatalf("Stop before Start = %v", err)
	}
	if err := l.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := l.Start(t.Context()); err == nil {
		t.Fatal("second Start succeeded")
	}
	if l.Name() != "public" || l.DrainPhase() != component.DrainIngress || l.Health(t.Context()).Status != component.StatusHealthy {
		t.Fatal("started listener metadata")
	}
	if code, _ := get(t, l.URL()+"/ok"); code != http.StatusNoContent {
		t.Fatalf("GET = %d", code)
	}
	for range 2 {
		if err := l.Quiesce(); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	l.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", http.NoBody))
	if rec.Code != http.StatusServiceUnavailable || l.Health(t.Context()).Status != component.StatusUnhealthy {
		t.Fatalf("quiesced listener answered %d", rec.Code)
	}
	if err := l.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := l.Stop(t.Context()); err != nil {
			t.Fatalf("Stop = %v", err)
		}
	}
}

func TestListenerDrainWaitsForInFlightRequests(t *testing.T) {
	t.Parallel()
	l := NewListener("public")
	entered, release := make(chan struct{}), make(chan struct{})
	l.Handle("GET /slow", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	if err := l.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	result := make(chan int, 1)
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, l.URL()+"/slow", http.NoBody)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			result <- 0
			return
		}
		_ = resp.Body.Close()
		result <- resp.StatusCode
	}()
	<-entered
	_ = l.Quiesce()
	drained := make(chan error, 1)
	go func() { drained <- l.Drain(context.Background()) }()
	select {
	case err := <-drained:
		t.Fatalf("Drain returned %v with a request in flight", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	if code := <-result; code != http.StatusOK {
		t.Fatalf("in-flight request = %d, want 200", code)
	}
	if err := l.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestListenerStopForceClosesAtDeadline(t *testing.T) {
	t.Parallel()
	l := NewListener("public")
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	l.Handle("GET /stuck", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	if err := l.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, l.URL()+"/stuck", http.NoBody)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := l.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop = %v, want DeadlineExceeded", err)
	}
}

func TestListenerDrainForceClosesAndCancelsRequestsAtDeadline(t *testing.T) {
	t.Parallel()
	l := NewListener("public")
	entered, finished := make(chan struct{}), make(chan struct{})
	l.Handle("GET /wait", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done() // a cooperative handler that only ends on cancellation
		close(finished)
	}))
	if err := l.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, l.URL()+"/wait", http.NoBody)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered
	_ = l.Quiesce()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := l.Drain(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Drain = %v, want the graceful deadline error", err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("Drain returned while the handler was still running")
	}
	if err := l.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestListenerShutdownWithoutDeadlineIsBounded(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"Drain", "Stop"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			l := NewListener("public")
			entered, finished := make(chan struct{}), make(chan struct{})
			release := make(chan struct{})
			l.Handle("/wait", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				defer close(finished)
				close(entered)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			if err := l.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			clientDone := make(chan struct{})
			t.Cleanup(func() {
				close(release)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := l.Stop(ctx); err != nil {
					t.Errorf("cleanup: %v", err)
				}
				awaitListener(t, clientDone)
			})
			go func() {
				defer close(clientDone)
				ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL()+"/wait", http.NoBody)
				if err != nil {
					t.Errorf("request: %v", err)
					return
				}
				if resp, err := http.DefaultClient.Do(req); err == nil {
					_ = resp.Body.Close()
				}
			}()
			awaitListener(t, entered)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if operation == "Stop" {
					done <- l.Stop(ctx)
				} else {
					done <- l.Drain(ctx)
				}
			}()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Errorf("%s = %v, want default graceful deadline error", operation, err)
				}
			case <-time.After(6 * time.Second):
				t.Errorf("%s did not enforce the five-second default budget", operation)
				cancel()
				awaitListener(t, done)
			}
			select {
			case <-finished:
			default:
				t.Error("shutdown returned before cooperative handler teardown")
			}
		})
	}
}

func awaitListener[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case got := <-result:
		return got
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for listener")
		var zero T
		return zero
	}
}

func TestListenerShutdownCancelsDirectRequests(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"Drain", "Stop", "CallerCancel"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			l := NewListener("public")
			type key struct{}
			parent, cancel := context.WithTimeout(context.WithValue(t.Context(), key{}, "request"), 3*time.Second)
			defer cancel()
			entered, finished := make(chan struct{}), make(chan struct{})
			l.Handle("/wait", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				if r.Context().Value(key{}) != "request" {
					t.Error("request context lost its values")
				}
				want, _ := parent.Deadline()
				if got, ok := r.Context().Deadline(); !ok || !got.Equal(want) {
					t.Error("request context lost its deadline")
				}
				close(entered)
				<-r.Context().Done()
			}))
			if err := l.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				cancel()
				ctx, stop := context.WithTimeout(context.Background(), time.Second)
				defer stop()
				if err := l.Stop(ctx); err != nil {
					t.Errorf("cleanup: %v", err)
				}
				awaitListener(t, finished)
			})
			go func() {
				defer close(finished)
				l.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(parent, http.MethodGet, "/wait", http.NoBody))
			}()
			awaitListener(t, entered)
			if operation == "CallerCancel" {
				cancel()
				awaitListener(t, finished)
			}
			ctx, stop := context.WithTimeout(t.Context(), time.Second)
			defer stop()
			shutdown := l.Drain
			if operation == "Stop" {
				shutdown = l.Stop
			}
			if err := shutdown(ctx); err != nil {
				t.Errorf("%s = %v, want cooperative direct request teardown", operation, err)
				cancel()
			}
			awaitListener(t, finished)
			rec := httptest.NewRecorder()
			l.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/wait", http.NoBody))
			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("request after shutdown = %d", rec.Code)
			}
		})
	}
}

func TestListenerDrainCancelsHijackedRequests(t *testing.T) {
	t.Parallel()
	l := NewListener("public")
	entered, finished := make(chan error, 1), make(chan struct{})
	release := make(chan struct{})
	l.Handle("/upgrade", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		conn, _, err := w.(http.Hijacker).Hijack()
		entered <- err
		if err != nil {
			return
		}
		defer func() {
			if err := conn.Close(); err != nil {
				t.Errorf("close hijacked connection: %v", err)
			}
		}()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	if err := l.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		close(release)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := l.Stop(ctx); err != nil {
			t.Errorf("cleanup: %v", err)
		}
		awaitListener(t, finished)
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(l.URL(), "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprint(conn, "GET /upgrade HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := awaitListener(t, entered); err != nil {
		t.Fatalf("Hijack = %v", err)
	}
	if err := l.Drain(ctx); err != nil {
		t.Errorf("Drain = %v, want cooperative hijacked request teardown", err)
	}
	select {
	case <-finished:
	default:
		t.Error("hijacked handler still running after Drain")
	}
}

// Not parallel: it inspects every goroutine, so no other test's listener may be draining.
func TestListenerDrainLeavesNoWaiterBehindAStuckHandler(t *testing.T) { //nolint:paralleltest // see above
	l := NewListener("public")
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	l.Handle("GET /stuck", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release // ignores cancellation, so it outlives every drain
	}))
	if err := l.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	go func() {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, l.URL()+"/stuck", http.NoBody)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-entered
	for range 3 {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		if err := l.Drain(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Drain = %v, want DeadlineExceeded", err)
		}
		cancel()
	}
	var dump strings.Builder
	if err := pprof.Lookup("goroutine").WriteTo(&dump, 1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dump.String(), "(*Listener).shutdown") {
		t.Fatalf("a drain left a goroutine waiting for the stuck handler:\n%s", dump.String())
	}
}

func TestListenerFallbackServesUnmatchedRequests(t *testing.T) {
	t.Parallel()
	l := NewListener("public")
	l.Handle("GET /ok", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	l.Fallback(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("spa")) }))
	for path, want := range map[string]string{"/ok": "ok", "/settings": "spa"} {
		rec := httptest.NewRecorder()
		l.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, http.NoBody))
		if rec.Body.String() != want {
			t.Errorf("GET %s = %q, want %q", path, rec.Body, want)
		}
	}
}
