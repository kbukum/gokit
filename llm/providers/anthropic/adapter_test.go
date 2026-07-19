package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

func TestNewAdapterExecuteInjectsHeadersAndDecodesResponse(t *testing.T) {
	var gotKey string
	var gotVersion string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"claude-test","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":2}}`))
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{BaseURL: srv.URL, APIKey: "unit-token", Model: "claude-test", APIVersion: "2026-01-01"})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	resp, err := adapter.Execute(context.Background(), llm.CompletionRequest{Messages: []chat.Message{chat.User("hi")}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotKey != "unit-token" || gotVersion != "2026-01-01" {
		t.Fatalf("headers key=%q version=%q", gotKey, gotVersion)
	}
	if gotBody["model"] != "claude-test" || resp.Text() != "hello" {
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
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
		flusher.Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	adapter, err := NewAdapter(Config{BaseURL: srv.URL, Model: "claude-test"})
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
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()
	adapter, err := NewAdapter(Config{BaseURL: srv.URL, Model: "claude-test"})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	_, err = adapter.Execute(context.Background(), llm.CompletionRequest{Messages: []chat.Message{chat.User("hi")}})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
}
