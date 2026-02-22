package testutil

import (
	"context"
	"net/http"
	"testing"
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
	srv.Start(context.Background())

	err := srv.Reset(context.Background())
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Should be able to start again
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
