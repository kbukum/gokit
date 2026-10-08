package server_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
