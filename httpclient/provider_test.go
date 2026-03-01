package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/provider"
	"github.com/kbukum/gokit/resilience"
)

func TestAdapter_Name(t *testing.T) {
	a, err := New(Config{Name: "my-http", BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Name(); got != "my-http" {
		t.Errorf("Name() = %q, want %q", got, "my-http")
	}
}

func TestAdapter_IsAvailable_NoCB(t *testing.T) {
	a, err := New(Config{BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if !a.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=true with no circuit breaker")
	}
}

func TestAdapter_IsAvailable_WithCB(t *testing.T) {
	cfg := resilience.DefaultCircuitBreakerConfig("test-cb")
	cfg.MaxFailures = 1
	a, err := New(Config{
		BaseURL:        "http://localhost",
		CircuitBreaker: &cfg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !a.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=true before failures")
	}
}

func TestAdapter_Execute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	a, err := New(Config{Name: "test-api", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := a.Execute(context.Background(), Request{
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

func TestAdapter_Execute_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	a, err := New(Config{Name: "test-api", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := a.Execute(context.Background(), Request{
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

func TestAdapter_Close(t *testing.T) {
	a, err := New(Config{BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(context.Background()); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestAdapter_GetConfig(t *testing.T) {
	a, err := New(Config{Name: "test", BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := a.GetConfig()
	if cfg.Name != "test" {
		t.Errorf("GetConfig().Name = %q, want %q", cfg.Name, "test")
	}
	if cfg.BaseURL != "http://localhost" {
		t.Errorf("GetConfig().BaseURL = %q, want %q", cfg.BaseURL, "http://localhost")
	}
}

func TestAdapter_ImplementsProvider(t *testing.T) {
	a, err := New(Config{Name: "test", BaseURL: "http://localhost"})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the adapter satisfies the provider interface
	var _ provider.RequestResponse[Request, *Response] = a
	var _ provider.Closeable = a
}
