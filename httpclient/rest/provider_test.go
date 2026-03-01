package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/httpclient"
)

func TestClient_ProviderInterface(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{Name: "test-rest", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Name
	if got := c.Name(); got != "test-rest" {
		t.Errorf("Name() = %q, want %q", got, "test-rest")
	}

	// IsAvailable
	if !c.IsAvailable(context.Background()) {
		t.Error("IsAvailable() = false, want true")
	}

	// Close
	if err := c.Close(context.Background()); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestClient_NameFromAdapter(t *testing.T) {
	a, err := httpclient.New(httpclient.Config{Name: "from-adapter"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c := NewFromAdapter(a)
	if got := c.Name(); got != "from-adapter" {
		t.Errorf("Name() = %q, want %q", got, "from-adapter")
	}
}

func TestErrors_Delegation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/not-found":
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		case "/unauthorized":
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		case "/rate-limited":
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]string{"error": "too many requests"})
		case "/server-error":
			w.WriteHeader(500)
			json.NewEncoder(w).Encode(map[string]string{"error": "internal"})
		}
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		path  string
		check func(error) bool
		name  string
	}{
		{"/not-found", IsNotFound, "IsNotFound"},
		{"/unauthorized", IsAuth, "IsAuth"},
		{"/rate-limited", IsRateLimit, "IsRateLimit"},
		{"/server-error", IsServerError, "IsServerError"},
		{"/server-error", IsRetryable, "IsRetryable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Get[map[string]string](context.Background(), c, tt.path)
			if err == nil {
				t.Fatal("expected error")
			}
			if !tt.check(err) {
				t.Errorf("%s(%v) = false, want true", tt.name, err)
			}
		})
	}
}
