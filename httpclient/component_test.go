package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComponent_Lifecycle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	comp := NewComponent(Config{
		Name:    "test-http",
		BaseURL: srv.URL,
	})

	// Before Start, adapter should be nil
	if comp.Adapter() != nil {
		t.Error("Adapter() should be nil before Start()")
	}

	// Start
	if err := comp.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// After Start, adapter should be available
	if comp.Adapter() == nil {
		t.Fatal("Adapter() should not be nil after Start()")
	}

	// Health should be healthy
	health := comp.Health(context.Background())
	if health.Status != "healthy" {
		t.Errorf("expected healthy, got %s", health.Status)
	}
	if health.Name != "test-http" {
		t.Errorf("expected name test-http, got %s", health.Name)
	}

	// Adapter should work
	resp, err := comp.Adapter().Do(context.Background(), Request{
		Method: http.MethodGet,
		Path:   "/",
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	// Stop
	if err := comp.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestComponent_Name_Default(t *testing.T) {
	comp := NewComponent(Config{BaseURL: "http://localhost"})
	if got := comp.Name(); got != "http" {
		t.Errorf("expected default name 'http', got %q", got)
	}
}

func TestComponent_Name_Custom(t *testing.T) {
	comp := NewComponent(Config{Name: "my-api", BaseURL: "http://localhost"})
	if got := comp.Name(); got != "my-api" {
		t.Errorf("expected 'my-api', got %q", got)
	}
}

func TestComponent_Describe(t *testing.T) {
	comp := NewComponent(Config{Name: "my-api", BaseURL: "http://example.com"})
	desc := comp.Describe()
	if desc.Type != "http-adapter" {
		t.Errorf("expected type 'http-adapter', got %q", desc.Type)
	}
	if desc.Details != "http://example.com" {
		t.Errorf("expected details with base URL, got %q", desc.Details)
	}
}

func TestComponent_Health_Unhealthy_BeforeStart(t *testing.T) {
	comp := NewComponent(Config{BaseURL: "http://localhost"})
	health := comp.Health(context.Background())
	if health.Status != "unhealthy" {
		t.Errorf("expected unhealthy before Start(), got %s", health.Status)
	}
}
