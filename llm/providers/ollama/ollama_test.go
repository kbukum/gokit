package ollama_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/providers/ollama"
)

func TestDialectRegisterAndName(t *testing.T) {
	registry := llm.NewDialectRegistry()
	if err := ollama.Register(registry); err != nil {
		t.Fatalf("register: %v", err)
	}
	d, err := registry.Get(ollama.DialectName)
	if err != nil {
		t.Fatalf("get dialect: %v", err)
	}
	if d.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", d.Name(), "ollama")
	}
	if d.ChatPath() != "/v1/chat/completions" {
		t.Errorf("ChatPath() = %q, want OpenAI-compatible /v1/chat/completions", d.ChatPath())
	}
}

func TestEndToEndChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q, want OpenAI-compatible path", r.URL.Path)
		}
		body, _ := json.Marshal(map[string]any{
			"id":    "chatcmpl-1",
			"model": "llama3.2",
			"choices": []map[string]any{{
				"message":       map[string]any{"content": "hello from ollama"},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	registry := llm.NewDialectRegistry()
	if err := ollama.Register(registry); err != nil {
		t.Fatalf("register: %v", err)
	}
	adapter, err := llm.New(registry, llm.Config{
		Dialect: ollama.DialectName,
		BaseURL: srv.URL,
		Model:   "llama3.2",
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	resp, err := adapter.Execute(context.Background(), llm.CompletionRequest{
		Messages: []chat.Message{chat.User("hi")},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(resp.Text(), "hello from ollama") {
		t.Errorf("response text = %q", resp.Text())
	}
}

func TestDefaultBaseURL(t *testing.T) {
	if ollama.DefaultBaseURL != "http://localhost:11434" {
		t.Errorf("DefaultBaseURL = %q, want http://localhost:11434", ollama.DefaultBaseURL)
	}
}
