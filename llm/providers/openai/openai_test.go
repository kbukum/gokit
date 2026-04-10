package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kbukum/gokit/llm"
)

// ---------------------------------------------------------------------------
// Dialect tests
// ---------------------------------------------------------------------------

func TestDialect_Name(t *testing.T) {
	d := &Dialect{}
	if d.Name() != "openai" {
		t.Fatalf("expected name 'openai', got %q", d.Name())
	}
}

func TestDialect_ChatPath(t *testing.T) {
	d := &Dialect{}
	if d.ChatPath() != "/v1/chat/completions" {
		t.Fatalf("unexpected chat path: %s", d.ChatPath())
	}
}

func TestDialect_StreamFormat(t *testing.T) {
	d := &Dialect{}
	if d.StreamFormat() != llm.StreamSSE {
		t.Fatal("expected StreamSSE")
	}
}

func TestDialect_BuildRequest_Basic(t *testing.T) {
	d := &Dialect{}
	req := llm.CompletionRequest{
		Model:        "gpt-4o",
		Messages:     []llm.Message{llm.User("hello")},
		SystemPrompt: "You are helpful.",
		Stream:       false,
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	if result["model"] != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %v", result["model"])
	}

	msgs, ok := result["messages"].([]any)
	if !ok || len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system + user), got %v", result["messages"])
	}

	sysMsg := msgs[0].(map[string]any)
	if sysMsg["role"] != "system" {
		t.Errorf("first message should be system, got %v", sysMsg["role"])
	}
}

func TestDialect_BuildRequest_WithToolChoice(t *testing.T) {
	d := &Dialect{}
	req := llm.CompletionRequest{
		Model:      "gpt-4o",
		Messages:   []llm.Message{llm.User("test")},
		ToolChoice: llm.ToolChoiceRequired,
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	if result["tool_choice"] != "required" {
		t.Errorf("expected tool_choice 'required', got %v", result["tool_choice"])
	}
}

func TestDialect_ParseResponse(t *testing.T) {
	d := &Dialect{}

	raw := `{
		"id": "chatcmpl-123",
		"model": "gpt-4o",
		"choices": [{
			"message": {"content": "Hello!"},
			"finish_reason": "stop"
		}],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 5,
			"total_tokens": 15
		}
	}`

	resp, err := d.ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}

	if resp.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", resp.Model)
	}
	if resp.Text() != "Hello!" {
		t.Errorf("expected text 'Hello!', got %q", resp.Text())
	}
	if resp.StopReason != llm.StopEndTurn {
		t.Errorf("expected StopEndTurn, got %v", resp.StopReason)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestDialect_ParseResponse_ToolCalls(t *testing.T) {
	d := &Dialect{}

	raw := `{
		"id": "chatcmpl-456",
		"model": "gpt-4o",
		"choices": [{
			"message": {
				"content": null,
				"tool_calls": [{
					"id": "call_abc",
					"type": "function",
					"function": {"name": "get_weather", "arguments": "{\"city\":\"NYC\"}"}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
	}`

	resp, err := d.ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}

	if !resp.HasToolCalls() {
		t.Fatal("expected tool calls")
	}
	if resp.Message.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %q", resp.Message.ToolCalls[0].Function.Name)
	}
	if resp.StopReason != llm.StopToolUse {
		t.Errorf("expected StopToolUse, got %v", resp.StopReason)
	}
}

func TestDialect_ParseStreamChunk_Content(t *testing.T) {
	d := &Dialect{}

	data := `{"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`
	chunk, err := d.ParseStreamChunk([]byte(data))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if chunk.Content != "Hello" {
		t.Errorf("expected content 'Hello', got %q", chunk.Content)
	}
	if chunk.Done {
		t.Error("expected done=false")
	}
}

func TestDialect_ParseStreamChunk_Done(t *testing.T) {
	d := &Dialect{}

	chunk, err := d.ParseStreamChunk([]byte("[DONE]"))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if chunk.Content != "" {
		t.Errorf("expected empty content, got %q", chunk.Content)
	}
	if !chunk.Done {
		t.Error("expected done=true")
	}
}

func TestDialect_ParseStreamChunk_FinishReason(t *testing.T) {
	d := &Dialect{}

	data := `{"choices":[{"delta":{"content":""},"finish_reason":"stop"}]}`
	chunk, err := d.ParseStreamChunk([]byte(data))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if !chunk.Done {
		t.Error("expected done=true for finish_reason=stop")
	}
}

func TestDialect_ParseStreamChunk_ToolCalls(t *testing.T) {
	d := &Dialect{}

	data := `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_123","type":"function","function":{"name":"search","arguments":"{\"q\":"}}]},"finish_reason":null}]}`
	chunk, err := d.ParseStreamChunk([]byte(data))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if len(chunk.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(chunk.ToolCalls))
	}
	tc := chunk.ToolCalls[0]
	if tc.ID != "call_123" {
		t.Errorf("tool call ID = %q, want %q", tc.ID, "call_123")
	}
	if tc.Function.Name != "search" {
		t.Errorf("tool call name = %q, want %q", tc.Function.Name, "search")
	}
	if tc.Function.Arguments != `{"q":` {
		t.Errorf("tool call args = %q, want %q", tc.Function.Arguments, `{"q":`)
	}
}

func TestDialect_RegisteredViaInit(t *testing.T) {
	d, err := llm.GetDialect("openai")
	if err != nil {
		t.Fatalf("GetDialect(openai): %v", err)
	}
	if d.Name() != "openai" {
		t.Errorf("expected openai dialect, got %q", d.Name())
	}
}

// ---------------------------------------------------------------------------
// Embedding provider tests
// ---------------------------------------------------------------------------

func TestEmbeddingProvider_EmptyBatch(t *testing.T) {
	p := NewEmbeddingProvider(DefaultConfig())
	result, err := p.EmbedBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestEmbeddingProvider_Dimensions(t *testing.T) {
	p := NewEmbeddingProvider(Config{EmbeddingDimensions: 768})
	if p.Dimensions() != 768 {
		t.Errorf("expected 768, got %d", p.Dimensions())
	}
}

func TestEmbeddingProvider_Embed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		if req["model"] != "text-embedding-3-small" {
			t.Errorf("unexpected model: %v", req["model"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{0.1, 0.2, 0.3}, "index": 0},
			},
		})
	}))
	defer srv.Close()

	p := NewEmbeddingProvider(Config{BaseURL: srv.URL})

	vec, err := p.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(vec))
	}
	if vec[0] < 0.09 || vec[0] > 0.11 {
		t.Errorf("expected ~0.1, got %f", vec[0])
	}
}

func TestEmbeddingProvider_EmbedBatch_Order(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{0.3, 0.3}, "index": 1},
				{"embedding": []float32{0.1, 0.1}, "index": 0},
			},
		})
	}))
	defer srv.Close()

	p := NewEmbeddingProvider(Config{BaseURL: srv.URL})

	results, err := p.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedBatch: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0][0] > 0.15 {
		t.Errorf("expected first result ~0.1, got %f", results[0][0])
	}
}

func TestEmbeddingProvider_AuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"embedding": []float32{0.1}, "index": 0},
			},
		})
	}))
	defer srv.Close()

	p := NewEmbeddingProvider(Config{BaseURL: srv.URL, APIKey: "sk-test"})
	_, _ = p.Embed(context.Background(), "test")

	if gotAuth != "Bearer sk-test" {
		t.Errorf("expected 'Bearer sk-test', got %q", gotAuth)
	}
}

func TestEmbeddingProvider_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	}))
	defer srv.Close()

	p := NewEmbeddingProvider(Config{BaseURL: srv.URL})
	_, err := p.Embed(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("unexpected base URL: %s", c.BaseURL)
	}
	if c.EmbeddingDimensions != 1536 {
		t.Errorf("unexpected dimensions: %d", c.EmbeddingDimensions)
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	c := Config{APIKey: "sk-test"}
	c.applyDefaults()
	if c.BaseURL == "" {
		t.Error("applyDefaults should set BaseURL")
	}
	if c.Model == "" {
		t.Error("applyDefaults should set Model")
	}
}
