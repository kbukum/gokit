package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/embedding"
	"github.com/kbukum/gokit/llm"
)

func TestNewAdapterExecuteInjectsAuthAndDecodesResponse(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-test","choices":[{"message":{"content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`))
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{BaseURL: srv.URL, APIKey: "unit-token", Model: "gpt-test"})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	resp, err := adapter.Execute(context.Background(), llm.CompletionRequest{Messages: []chat.Message{chat.User("hi")}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotAuth != "Bearer unit-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody["model"] != "gpt-test" || resp.Text() != "hello" {
		t.Fatalf("body=%#v resp=%#v", gotBody, resp)
	}
}

func TestNewAdapterStreamAssemblesResponseAndHonorsCancellation(t *testing.T) {
	requestDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(requestDone)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected flusher")
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{BaseURL: srv.URL, Model: "gpt-test"})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	events, err := adapter.Stream(ctx, llm.CompletionRequest{Messages: []chat.Message{chat.User("hi")}})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	event := <-events
	delta, ok := event.(llm.TextDelta)
	if !ok || delta.Text != "hello" {
		t.Fatalf("event = %#v", event)
	}
	cancel()
	for range events {
	}
	<-requestDone
}

func TestNewAdapterPropagatesHTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()
	adapter, err := NewAdapter(Config{BaseURL: srv.URL, Model: "gpt-test"})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	_, err = adapter.Execute(context.Background(), llm.CompletionRequest{Messages: []chat.Message{chat.User("hi")}})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
}

func TestEmbeddingProviderLifecycleHealthModelOptionsAndMalformedResponse(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad/embeddings" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{`))
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.4,0.5],"index":0}],"usage":{"prompt_tokens":3,"total_tokens":7}}`))
	}))
	defer srv.Close()

	p, err := NewEmbeddingProvider(Config{BaseURL: srv.URL, EmbeddingModel: "default-embed"})
	if err != nil {
		t.Fatalf("NewEmbeddingProvider: %v", err)
	}
	if p.Name() != "openai-embedding" {
		t.Fatalf("Name = %q", p.Name())
	}
	if p.Health(context.Background()).Status != component.StatusDegraded {
		t.Fatal("expected degraded before Start")
	}
	if err := p.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	resp, err := p.Execute(context.Background(), embedding.EmbedRequest{
		Model:   ai.Model{Provider: ai.ProviderOpenAI, Name: "custom-embed"},
		Inputs:  []embedding.EmbedInput{embedding.Text{Text: "hello"}},
		Options: map[string]any{"encoding_format": "float"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotBody["model"] != "custom-embed" || gotBody["encoding_format"] != "float" {
		t.Fatalf("request body = %#v", gotBody)
	}
	if resp.Usage.InputTokens != 3 || resp.Usage.OutputTokens != 4 {
		t.Fatalf("usage = %#v", resp.Usage)
	}
	if p.Health(context.Background()).Status != component.StatusHealthy {
		t.Fatalf("health = %#v", p.Health(context.Background()))
	}
	if !p.IsAvailable(context.Background()) {
		t.Fatal("expected provider available")
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	bad, err := NewEmbeddingProvider(Config{BaseURL: srv.URL + "/bad"})
	if err != nil {
		t.Fatalf("NewEmbeddingProvider bad: %v", err)
	}
	_, err = bad.Execute(context.Background(), embedding.EmbedRequest{Inputs: []embedding.EmbedInput{embedding.Text{Text: "bad"}}})
	if err == nil {
		t.Fatal("expected malformed embedding response error")
	}
}
