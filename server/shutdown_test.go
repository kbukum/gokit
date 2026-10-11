package server_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/kbukum/gokit/server"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestDiagnosticsAreNeverPublic(t *testing.T) {
	s := server.New(&server.Config{Host: "127.0.0.1"}, nil)
	s.ApplyDefaults("test", nil)
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Stop(context.Background()); err != nil {
			t.Error(err)
		}
	})
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	for _, endpoint := range []string{"/metrics", "/debug/pprof/"} {
		response, err := client.Get("http://" + s.ListenAddr().String() + endpoint)
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		if err := errors.Join(readErr, response.Body.Close()); err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("public %s = %d, want 404", endpoint, response.StatusCode)
		}
	}
}

func TestStopDoesNotWaitForConnectionsThatSentNothing(t *testing.T) {
	s := server.New(&server.Config{Host: "127.0.0.1"}, nil)
	s.ApplyMiddleware()
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	// A preconnect or probe: accepted, but no request byte yet. net/http counts it active for five seconds.
	silent, err := net.Dial("tcp", s.ListenAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = silent.Close() }()
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://" + s.ListenAddr().String() + "/missing")
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := s.Stop(ctx); err != nil {
		t.Fatalf("a drain that lost no request reported %v", err)
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Fatalf("drain waited %s on a connection that sent nothing", took)
	}
	if err := silent.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := silent.Read(make([]byte, 1)); !errors.Is(err, io.EOF) && !errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("silent connection read = %v, want closed", err)
	}
}

func TestStopForceClosesCooperativeHandler(t *testing.T) {
	s := server.New(&server.Config{Host: "127.0.0.1"}, nil)
	started, finished := make(chan struct{}), make(chan struct{})
	s.Handle("/slow", http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(finished)
	}))
	s.ApplyMiddleware()
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	done := make(chan struct{})
	go func() {
		defer close(done)
		response, err := client.Get("http://" + s.ListenAddr().String() + "/slow")
		if err == nil {
			if err := response.Body.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	<-started
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := s.Stop(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("forced shutdown must report deadline: %v", err)
	}
	select {
	case <-finished:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("forced close left the handler running")
	}
	<-done
	if time.Since(start) > 350*time.Millisecond {
		t.Fatal("shutdown exceeded budget plus 250ms scheduling allowance")
	}
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", http.NoBody))
	if response.Code != 503 {
		t.Fatalf("shutdown readiness = %d", response.Code)
	}
}
