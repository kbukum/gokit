package testutil

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	srv := NewServer()
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.BaseURL() != "" {
		t.Error("expected empty base URL before start")
	}
}

func TestServerStartStop(t *testing.T) {
	srv := NewServer()

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer srv.Stop(context.Background())

	url := srv.BaseURL()
	if url == "" {
		t.Fatal("expected non-empty base URL after start")
	}

	// Health should be healthy
	h := srv.Health(context.Background())
	if h.Status != "healthy" {
		t.Errorf("expected healthy, got %s", h.Status)
	}

	// Stop
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if srv.BaseURL() != "" {
		t.Error("expected empty base URL after stop")
	}
}

func TestServerDoubleStart(t *testing.T) {
	srv := NewServer()
	srv.Start(context.Background())
	defer srv.Stop(context.Background())

	err := srv.Start(context.Background())
	if err == nil {
		t.Error("expected error on double start")
	}
}

func TestServerMount(t *testing.T) {
	srv := NewServer()

	// Mount a simple handler
	srv.Mount("/test/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	srv.Start(context.Background())
	defer srv.Stop(context.Background())

	// Make a request
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.BaseURL()+"/test/hello", http.NoBody)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServerReset(t *testing.T) {
	srv := NewServer()
	srv.Mount("/kept", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	srv.Start(context.Background())
	defer srv.Stop(context.Background())
	origin := srv.BaseURL()

	err := srv.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	if srv.BaseURL() != origin {
		t.Fatal("reset replaced the server")
	}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, origin+"/kept", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset lost mounted handler: %d", resp.StatusCode)
	}
	if err := srv.Stop(t.Context()); err != nil {
		t.Fatal(err)
	}
	// Explicit stop/start owns restart without losing handlers.
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start after reset failed: %v", err)
	}
	defer srv.Stop(context.Background())

	if srv.BaseURL() == "" {
		t.Error("expected non-empty base URL after restart")
	}
}

func TestServerClient(t *testing.T) {
	srv := NewServer()
	if srv.Client() == http.DefaultClient {
		t.Log("Client returns default before start (expected)")
	}

	srv.Start(context.Background())
	defer srv.Stop(context.Background())

	client := srv.Client()
	if client == nil {
		t.Fatal("expected non-nil client after start")
	}
	if client == http.DefaultClient {
		t.Error("expected test-specific client, not default")
	}
}

func TestStopAllowsCanceledHandlerToReadState(t *testing.T) {
	t.Parallel()
	s := NewServer()
	exited := make(chan struct{})
	s.Mount("/stream", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(exited)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		s.Health(context.Background())
		s.BaseURL()
		s.Client()
	}))
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL()+"/stream", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	stopCtx, stopCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stopCancel()
	stopped := make(chan error, 1)
	go func() { stopped <- s.Stop(stopCtx) }()
	select {
	case err := <-stopped:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("drain failure: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("teardown deadlocked a cancellation-cooperative handler")
	}
	select {
	case <-exited:
	case <-ctx.Done():
		t.Fatal("handler retained")
	}
}
