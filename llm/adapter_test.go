package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- mock dialect for testing ---

type mockDialect struct {
	name         string
	chatPath     string
	healthPath   string
	streamFormat StreamFormat
	buildErr     error
	parseErr     error
}

func (d *mockDialect) Name() string {
	if d.name != "" {
		return d.name
	}
	return "mock"
}

func (d *mockDialect) ChatPath() string {
	if d.chatPath != "" {
		return d.chatPath
	}
	return "/chat"
}

func (d *mockDialect) HealthPath() string { return d.healthPath }

func (d *mockDialect) BuildRequest(req CompletionRequest) (any, error) {
	if d.buildErr != nil {
		return nil, d.buildErr
	}
	return map[string]any{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   req.Stream,
	}, nil
}

func (d *mockDialect) ParseResponse(body []byte) (*CompletionResponse, error) {
	if d.parseErr != nil {
		return nil, d.parseErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	content, _ := raw["content"].(string)
	model, _ := raw["model"].(string)
	return &CompletionResponse{
		Content: content,
		Model:   model,
		Usage:   Usage{TotalTokens: 10},
	}, nil
}

func (d *mockDialect) StreamFormat() StreamFormat { return d.streamFormat }

func (d *mockDialect) ParseStreamChunk(data []byte) (content string, done bool, err error) {
	var chunk struct {
		Content string `json:"content"`
		Done    bool   `json:"done"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil {
		return "", false, err
	}
	return chunk.Content, chunk.Done, nil
}

// --- tests ---

func TestAdapter_New_WithDialect(t *testing.T) {
	// Register a mock dialect
	dialectsMu.Lock()
	original := dialects
	dialects = map[string]Dialect{}
	dialectsMu.Unlock()
	defer func() {
		dialectsMu.Lock()
		dialects = original
		dialectsMu.Unlock()
	}()

	RegisterDialect("mock", &mockDialect{})

	a, err := New(Config{
		Dialect: "mock",
		BaseURL: "http://localhost:12345",
		Model:   "test-model",
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if a.Name() != "mock-llm" {
		t.Errorf("Name() = %q, want %q", a.Name(), "mock-llm")
	}
	if a.Dialect().Name() != "mock" {
		t.Errorf("Dialect().Name() = %q, want %q", a.Dialect().Name(), "mock")
	}
}

func TestAdapter_New_UnknownDialect(t *testing.T) {
	_, err := New(Config{Dialect: "nonexistent-xyz"})
	if err == nil {
		t.Fatal("expected error for unknown dialect")
	}
}

func TestAdapter_NewWithDialect(t *testing.T) {
	d := &mockDialect{name: "direct"}
	a, err := NewWithDialect(d, Config{
		BaseURL: "http://localhost:12345",
		Model:   "test-model",
	})
	if err != nil {
		t.Fatalf("NewWithDialect() error: %v", err)
	}
	if a.Name() != "direct-llm" {
		t.Errorf("Name() = %q, want %q", a.Name(), "direct-llm")
	}
}

func TestAdapter_NewWithDialect_NilDialect(t *testing.T) {
	_, err := NewWithDialect(nil, Config{})
	if err != ErrNoDialect {
		t.Errorf("expected ErrNoDialect, got %v", err)
	}
}

func TestAdapter_Execute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat" {
			t.Errorf("path = %q, want /chat", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		// Verify the request body was built by the dialect
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "test-model" {
			t.Errorf("model = %v, want test-model", body["model"])
		}

		json.NewEncoder(w).Encode(map[string]any{
			"content": "Hello from LLM!",
			"model":   "test-model",
		})
	}))
	defer srv.Close()

	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{
		BaseURL: srv.URL,
		Model:   "test-model",
	})
	if err != nil {
		t.Fatalf("NewWithDialect() error: %v", err)
	}

	resp, err := a.Execute(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if resp.Content != "Hello from LLM!" {
		t.Errorf("Content = %q, want %q", resp.Content, "Hello from LLM!")
	}
	if resp.Model != "test-model" {
		t.Errorf("Model = %q, want %q", resp.Model, "test-model")
	}
}

func TestAdapter_Execute_AppliesDefaults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		// Defaults should be applied
		if body["model"] != "default-model" {
			t.Errorf("model = %v, want default-model", body["model"])
		}

		json.NewEncoder(w).Encode(map[string]any{"content": "ok", "model": "default-model"})
	}))
	defer srv.Close()

	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{
		BaseURL:     srv.URL,
		Model:       "default-model",
		Temperature: 0.7,
		MaxTokens:   100,
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// Send request WITHOUT model/temp/max — defaults should apply
	_, err = a.Execute(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
}

func TestAdapter_Execute_BuildError(t *testing.T) {
	d := &mockDialect{buildErr: fmt.Errorf("build failed")}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	_, err = a.Execute(context.Background(), CompletionRequest{})
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Errorf("expected build request error, got %v", err)
	}
}

func TestAdapter_IsAvailable_WithHealthPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	d := &mockDialect{healthPath: "/health"}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if !a.IsAvailable(context.Background()) {
		t.Error("IsAvailable() = false, want true")
	}
}

func TestAdapter_IsAvailable_NoHealthPath(t *testing.T) {
	d := &mockDialect{healthPath: ""}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	// Without health path, delegates to REST client which checks circuit breaker state
	if !a.IsAvailable(context.Background()) {
		t.Error("IsAvailable() = false, want true (no CB configured)")
	}
}

func TestAdapter_Close(t *testing.T) {
	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if err := a.Close(context.Background()); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestAdapter_Stream_NDJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		chunks := []string{
			`{"content":"Hello","done":false}`,
			`{"content":" world","done":false}`,
			`{"content":"","done":true}`,
		}
		for _, chunk := range chunks {
			fmt.Fprintln(w, chunk)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	d := &mockDialect{streamFormat: StreamNDJSON}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	ch, err := a.Stream(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var content strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		content.WriteString(chunk.Content)
		if chunk.Done {
			break
		}
	}

	if got := content.String(); got != "Hello world" {
		t.Errorf("streamed content = %q, want %q", got, "Hello world")
	}
}

func TestAdapter_Stream_SSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}

		events := []string{
			"data: {\"content\":\"Hello\",\"done\":false}\n\n",
			"data: {\"content\":\" there\",\"done\":false}\n\n",
			"data: {\"content\":\"\",\"done\":true}\n\n",
		}
		for _, event := range events {
			io.WriteString(w, event)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	d := &mockDialect{streamFormat: StreamSSE}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	ch, err := a.Stream(context.Background(), CompletionRequest{
		Messages: []Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	var content strings.Builder
	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("stream error: %v", chunk.Err)
		}
		content.WriteString(chunk.Content)
		if chunk.Done {
			break
		}
	}

	if got := content.String(); got != "Hello there" {
		t.Errorf("streamed content = %q, want %q", got, "Hello there")
	}
}

func TestAdapter_REST_Returns_Client(t *testing.T) {
	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{BaseURL: "http://localhost:1"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	if a.REST() == nil {
		t.Error("REST() returned nil")
	}
}
