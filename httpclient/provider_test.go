package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/resilience"
)

func TestClientProvider_Name(t *testing.T) {
	c, err := New(Config{BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	p := NewProvider("my-http", c)
	if got := p.Name(); got != "my-http" {
		t.Errorf("Name() = %q, want %q", got, "my-http")
	}
}

func TestClientProvider_IsAvailable_NoCB(t *testing.T) {
	c, err := New(Config{BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	p := NewProvider("test", c)
	if !p.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=true with no circuit breaker")
	}
}

func TestClientProvider_IsAvailable_WithCB(t *testing.T) {
	cfg := resilience.DefaultCircuitBreakerConfig("test-cb")
	cfg.MaxFailures = 1
	c, err := New(Config{
		BaseURL:        "http://localhost",
		CircuitBreaker: &cfg,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := NewProvider("test", c)
	if !p.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=true before failures")
	}
}

func TestClientProvider_Execute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	p := NewProvider("test-api", c)

	resp, err := p.Execute(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/health",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestClientProvider_Execute_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	p := NewProvider("test-api", c)

	resp, err := p.Execute(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/fail",
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if resp == nil || resp.StatusCode != 500 {
		t.Errorf("expected resp with status 500, got %v", resp)
	}
}

func TestClientProvider_Client(t *testing.T) {
	c, err := New(Config{BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	p := NewProvider("test", c)
	if p.Client() != c {
		t.Error("Client() should return the underlying client")
	}
}
