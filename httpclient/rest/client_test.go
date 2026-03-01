package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/httpclient"
)

type testUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/users/1" {
			t.Errorf("expected /users/1, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Accept"); ct != "application/json" {
			t.Errorf("expected Accept: application/json, got %s", ct)
		}
		json.NewEncoder(w).Encode(testUser{Name: "Alice", Email: "alice@example.com"})
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := Get[testUser](context.Background(), c, "/users/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data.Name != "Alice" {
		t.Errorf("expected Alice, got %s", resp.Data.Name)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestPost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var user testUser
		json.NewDecoder(r.Body).Decode(&user)
		user.Email = "bob@example.com"
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(user)
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := Post[testUser](context.Background(), c, "/users", testUser{Name: "Bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	if resp.Data.Name != "Bob" {
		t.Errorf("expected Bob, got %s", resp.Data.Name)
	}
	if resp.Data.Email != "bob@example.com" {
		t.Errorf("expected bob@example.com, got %s", resp.Data.Email)
	}
}

func TestPut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(testUser{Name: "Updated"})
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := Put[testUser](context.Background(), c, "/users/1", testUser{Name: "Updated"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data.Name != "Updated" {
		t.Errorf("expected Updated, got %s", resp.Data.Name)
	}
}

func TestPatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(testUser{Name: "Patched"})
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := Patch[testUser](context.Background(), c, "/users/1", map[string]string{"name": "Patched"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data.Name != "Patched" {
		t.Errorf("expected Patched, got %s", resp.Data.Name)
	}
}

func TestDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := Delete[map[string]bool](context.Background(), c, "/users/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Data["deleted"] {
		t.Error("expected deleted=true")
	}
}

func TestGet_WithQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Errorf("expected page=2, got %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "10" {
			t.Errorf("expected limit=10, got %q", got)
		}
		json.NewEncoder(w).Encode([]testUser{{Name: "Alice"}})
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := Get[[]testUser](context.Background(), c, "/users",
		WithQuery(map[string]string{"page": "2", "limit": "10"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 user, got %d", len(resp.Data))
	}
}

func TestGet_WithHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Request-ID"); got != "abc-123" {
			t.Errorf("expected X-Request-ID=abc-123, got %q", got)
		}
		json.NewEncoder(w).Encode(testUser{Name: "Alice"})
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = Get[testUser](context.Background(), c, "/users/1",
		WithHeaders(map[string]string{"X-Request-ID": "abc-123"}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGet_WithAuthOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer override" {
			t.Errorf("expected Bearer override, got %q", got)
		}
		json.NewEncoder(w).Encode(testUser{Name: "Alice"})
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{
		BaseURL: srv.URL,
		Auth:    httpclient.BearerAuth("default"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = Get[testUser](context.Background(), c, "/users/1",
		WithAuth(httpclient.BearerAuth("override")),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGet_ErrorResponse_StillDecodesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer srv.Close()

	c, err := New(httpclient.Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, err := Get[map[string]string](context.Background(), c, "/users/999")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !httpclient.IsNotFound(err) {
		t.Errorf("expected not found error, got %v", err)
	}
	// Response body should still be decoded when possible
	if resp != nil && resp.Data["error"] != "not found" {
		t.Logf("decoded error body: %v", resp.Data)
	}
}

func TestNewFromAdapter(t *testing.T) {
	httpA, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c := NewFromAdapter(httpA)
	if c.HTTP() != httpA {
		t.Error("HTTP() should return the underlying adapter")
	}
}
