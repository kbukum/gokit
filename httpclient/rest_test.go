package httpclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type testItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/items/1" {
			t.Errorf("expected /items/1, got %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(testItem{ID: 1, Name: "Widget"})
	}))
	defer srv.Close()

	a, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := Get[testItem](a, context.Background(), "/items/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Data.Name != "Widget" {
		t.Errorf("expected Widget, got %s", resp.Data.Name)
	}
}

func TestPost_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		var item testItem
		json.NewDecoder(r.Body).Decode(&item)
		item.ID = 42
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(item)
	}))
	defer srv.Close()

	a, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := Post[testItem](a, context.Background(), "/items", testItem{Name: "Gadget"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 201 {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	if resp.Data.ID != 42 {
		t.Errorf("expected ID=42, got %d", resp.Data.ID)
	}
}

func TestPut_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(testItem{ID: 1, Name: "Updated"})
	}))
	defer srv.Close()

	a, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := Put[testItem](a, context.Background(), "/items/1", testItem{Name: "Updated"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data.Name != "Updated" {
		t.Errorf("expected Updated, got %s", resp.Data.Name)
	}
}

func TestPatch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(testItem{ID: 1, Name: "Patched"})
	}))
	defer srv.Close()

	a, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := Patch[testItem](a, context.Background(), "/items/1", map[string]string{"name": "Patched"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Data.Name != "Patched" {
		t.Errorf("expected Patched, got %s", resp.Data.Name)
	}
}

func TestDelete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		json.NewEncoder(w).Encode(map[string]bool{"deleted": true})
	}))
	defer srv.Close()

	a, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := Delete[map[string]bool](a, context.Background(), "/items/1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Data["deleted"] {
		t.Error("expected deleted=true")
	}
}

func TestGet_WithOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Errorf("expected page=2, got %q", got)
		}
		if got := r.Header.Get("X-Trace"); got != "abc" {
			t.Errorf("expected X-Trace=abc, got %q", got)
		}
		json.NewEncoder(w).Encode([]testItem{{ID: 1, Name: "Alice"}})
	}))
	defer srv.Close()

	a, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := Get[[]testItem](a, context.Background(), "/items",
		WithQueryParam("page", "2"),
		WithHeader("X-Trace", "abc"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Data))
	}
}

func TestGet_WithAuthOption(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %q", got)
		}
		json.NewEncoder(w).Encode(testItem{ID: 1, Name: "Alice"})
	}))
	defer srv.Close()

	a, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	_, err = Get[testItem](a, context.Background(), "/items/1",
		WithRequestAuth(BearerAuth("test-token")),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGet_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer srv.Close()

	a, err := New(Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}

	resp, err := Get[map[string]string](a, context.Background(), "/items/999")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !IsNotFound(err) {
		t.Errorf("expected not found error, got %v", err)
	}
	if resp != nil && resp.Data["error"] != "not found" {
		t.Errorf("expected decoded error body")
	}
}
