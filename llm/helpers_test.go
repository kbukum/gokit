package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/provider"
)

// Verify helper functions accept the provider.RequestResponse interface.
var _ provider.RequestResponse[CompletionRequest, CompletionResponse] = (*Adapter)(nil)

func TestComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"content": "The answer is 42.",
			"model":   "test",
		})
	}))
	defer srv.Close()

	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL, Model: "test"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	result, err := Complete(context.Background(), a, "You are helpful.", "What is the answer?")
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if result != "The answer is 42." {
		t.Errorf("result = %q, want %q", result, "The answer is 42.")
	}
}

func TestCompleteStructured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"content": `{"name": "Alice", "age": 30}`,
			"model":   "test",
		})
	}))
	defer srv.Close()

	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL, Model: "test"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var result struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	err = CompleteStructured(context.Background(), a, "Extract info.", "Alice is 30.", &result)
	if err != nil {
		t.Fatalf("CompleteStructured() error: %v", err)
	}
	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("result = %+v, want {Alice 30}", result)
	}
}

func TestCompleteStructured_WithMarkdownFence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"content": "```json\n{\"name\": \"Bob\"}\n```",
			"model":   "test",
		})
	}))
	defer srv.Close()

	d := &mockDialect{}
	a, err := NewWithDialect(d, Config{BaseURL: srv.URL, Model: "test"})
	if err != nil {
		t.Fatalf("error: %v", err)
	}

	var result struct {
		Name string `json:"name"`
	}
	err = CompleteStructured(context.Background(), a, "Extract.", "Bob", &result)
	if err != nil {
		t.Fatalf("CompleteStructured() error: %v", err)
	}
	if result.Name != "Bob" {
		t.Errorf("Name = %q, want %q", result.Name, "Bob")
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain json", `{"key": "value"}`, `{"key": "value"}`},
		{"with whitespace", `  {"key": "value"}  `, `{"key": "value"}`},
		{"markdown fence", "```json\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"with prefix text", `Here is the result: {"key": "value"}`, `{"key": "value"}`},
		{"no json", "just text", "just text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.input)
			if got != tt.want {
				t.Errorf("extractJSON(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
