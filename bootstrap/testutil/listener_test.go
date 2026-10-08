package testutil

import (
	"context"
	"errors"
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
