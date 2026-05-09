package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm/internal/streamwire"
)

type testDialect struct{}

func (testDialect) Name() string       { return "test" }
func (testDialect) ChatPath() string   { return "/chat" }
func (testDialect) HealthPath() string { return "/health" }
func (testDialect) BuildRequest(req CompletionRequest) (any, error) {
	return map[string]any{"model": req.Model, "stream": req.Stream}, nil
}

func (testDialect) ParseResponse([]byte) (*CompletionResponse, error) {
	r := CompletionResponse{Message: chat.Assistant("ok"), Model: "m", StopReason: chat.FinishReasonStop}
	return &r, nil
}
func (testDialect) StreamFormat() StreamFormat { return StreamNDJSON }
func (testDialect) ParseStreamChunk(data []byte) (streamwire.Chunk, error) {
	var v struct {
		Content string `json:"content"`
		Done    bool   `json:"done"`
		Tool    string `json:"tool"`
		Args    string `json:"args"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return streamwire.Chunk{}, err
	}
	chunk := streamwire.Chunk{Content: v.Content, Done: v.Done}
	if v.Tool != "" {
		chunk.ToolCalls = []streamwire.ToolCall{{Index: 0, ID: "1", Name: v.Tool, InputDelta: v.Args}}
	}
	return chunk, nil
}

func TestAdapterProviderCompleteStreamAndCapabilities(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["stream"] != true {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"ok"}`))
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"content\":\"he\"}\n{\"tool\":\"calc\",\"args\":\"{\\\"a\\\":\"}\n{\"tool\":\"calc\",\"args\":\"1}\",\"done\":true}\n"))
	}))
	defer srv.Close()
	adapter, err := NewWithDialect(testDialect{}, Config{BaseURL: srv.URL, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	provider := NewProvider(adapter, "m").WithCapabilities(Capabilities{MaxInputTokens: 128000}).WithDefaults(func(req *CompletionRequest) { req.MaxTokens = 7 })
	if !adapter.IsAvailable(context.Background()) {
		t.Fatal("expected available")
	}
	if provider.Capabilities().MaxInputTokens != 128000 {
		t.Fatal("capabilities not set")
	}
	if provider.CountTokens([]chat.Message{chat.User("hello")}) == 0 {
		t.Fatal("expected token count")
	}
	resp, err := provider.Execute(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text() != "ok" {
		t.Fatalf("resp=%s", resp.Text())
	}
	ch, err := provider.Stream(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var complete bool
	for evt := range ch {
		if mc, ok := evt.(MessageComplete); ok {
			complete = true
			if len(mc.Response.Message.ToolCalls) != 1 {
				t.Fatalf("tool calls=%d", len(mc.Response.Message.ToolCalls))
			}
		}
	}
	if !complete {
		t.Fatal("missing complete")
	}
}
