package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/llm"
)

func TestDialect_Name(t *testing.T) {
	d := &Dialect{}
	if d.Name() != "anthropic" {
		t.Fatalf("expected name 'anthropic', got %q", d.Name())
	}
}

func TestDialect_ChatPath(t *testing.T) {
	d := &Dialect{}
	if d.ChatPath() != "/v1/messages" {
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
		Model:        "claude-sonnet-4-20250514",
		Messages:     []llm.Message{llm.User("hello")},
		SystemPrompt: "You are helpful.",
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	if result["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("expected model claude-sonnet-4-20250514, got %v", result["model"])
	}
	if result["system"] != "You are helpful." {
		t.Errorf("expected system prompt at top level, got %v", result["system"])
	}
	if _, ok := result["max_tokens"]; !ok {
		t.Error("expected max_tokens to be set (Anthropic requires it)")
	}
}

func TestDialect_BuildRequest_WithMaxTokens(t *testing.T) {
	d := &Dialect{}
	req := llm.CompletionRequest{
		Model:     "claude-sonnet-4-20250514",
		Messages:  []llm.Message{llm.User("test")},
		MaxTokens: 1000,
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	if int(result["max_tokens"].(float64)) != 1000 {
		t.Errorf("expected max_tokens=1000, got %v", result["max_tokens"])
	}
}

func TestDialect_BuildRequest_WithToolChoice(t *testing.T) {
	d := &Dialect{}
	req := llm.CompletionRequest{
		Model:      "claude-sonnet-4-20250514",
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

	tc, ok := result["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_choice to be object, got %T", result["tool_choice"])
	}
	if tc["type"] != "any" {
		t.Errorf("expected tool_choice type 'any' for required, got %v", tc["type"])
	}
}

func TestDialect_ParseResponse_Text(t *testing.T) {
	d := &Dialect{}

	raw := `{
		"id": "msg_123",
		"model": "claude-sonnet-4-20250514",
		"content": [{"type": "text", "text": "Hello!"}],
		"stop_reason": "end_turn",
		"usage": {"input_tokens": 10, "output_tokens": 5}
	}`

	resp, err := d.ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}

	if resp.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model claude-sonnet-4-20250514, got %s", resp.Model)
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

func TestDialect_ParseResponse_ToolUse(t *testing.T) {
	d := &Dialect{}

	raw := `{
		"id": "msg_456",
		"model": "claude-sonnet-4-20250514",
		"content": [
			{"type": "text", "text": "I'll check the weather."},
			{"type": "tool_use", "id": "toolu_abc", "name": "get_weather", "input": {"city": "NYC"}}
		],
		"stop_reason": "tool_use",
		"usage": {"input_tokens": 20, "output_tokens": 10}
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

func TestDialect_ParseStreamChunk_ContentDelta(t *testing.T) {
	d := &Dialect{}

	data := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`
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

func TestDialect_ParseStreamChunk_MessageStop(t *testing.T) {
	d := &Dialect{}

	data := `{"type":"message_stop"}`
	chunk, err := d.ParseStreamChunk([]byte(data))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if !chunk.Done {
		t.Error("expected done=true for message_stop")
	}
}

func TestDialect_ParseStreamChunk_Ping(t *testing.T) {
	d := &Dialect{}

	data := `{"type":"ping"}`
	chunk, err := d.ParseStreamChunk([]byte(data))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if chunk.Content != "" {
		t.Errorf("expected empty content for ping, got %q", chunk.Content)
	}
	if chunk.Done {
		t.Error("expected done=false for ping")
	}
}

func TestDialect_ParseStreamChunk_ToolUse(t *testing.T) {
	d := &Dialect{}

	// content_block_start with tool_use type
	data := `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_123","name":"search"}}`
	chunk, err := d.ParseStreamChunk([]byte(data))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if len(chunk.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(chunk.ToolCalls))
	}
	if chunk.ToolCalls[0].ID != "toolu_123" {
		t.Errorf("tool ID = %q, want %q", chunk.ToolCalls[0].ID, "toolu_123")
	}
	if chunk.ToolCalls[0].Function.Name != "search" {
		t.Errorf("tool name = %q, want %q", chunk.ToolCalls[0].Function.Name, "search")
	}

	// input_json_delta
	data2 := `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"test\"}"}}`
	chunk2, err := d.ParseStreamChunk([]byte(data2))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if len(chunk2.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call delta, got %d", len(chunk2.ToolCalls))
	}
	if chunk2.ToolCalls[0].Function.Arguments != `{"q":"test"}` {
		t.Errorf("tool args = %q, want %q", chunk2.ToolCalls[0].Function.Arguments, `{"q":"test"}`)
	}
}

func TestDialect_RegisteredViaInit(t *testing.T) {
	d, err := llm.GetDialect("anthropic")
	if err != nil {
		t.Fatalf("GetDialect(anthropic): %v", err)
	}
	if d.Name() != "anthropic" {
		t.Errorf("expected anthropic dialect, got %q", d.Name())
	}
}

func TestDialect_ToolResultEncoding(t *testing.T) {
	d := &Dialect{}
	req := llm.CompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []llm.Message{
			llm.User("what's the weather?"),
			llm.ToolResultMsg("toolu_abc", "72°F and sunny", false),
		},
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	msgs := result["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	toolMsg := msgs[1].(map[string]any)
	if toolMsg["role"] != "user" {
		t.Errorf("Anthropic tool results use role=user, got %v", toolMsg["role"])
	}

	content := toolMsg["content"].([]any)
	block := content[0].(map[string]any)
	if block["type"] != "tool_result" {
		t.Errorf("expected type 'tool_result', got %v", block["type"])
	}
	if block["tool_use_id"] != "toolu_abc" {
		t.Errorf("expected tool_use_id 'toolu_abc', got %v", block["tool_use_id"])
	}
}

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.BaseURL != "https://api.anthropic.com" {
		t.Errorf("unexpected base URL: %s", c.BaseURL)
	}
	if c.APIVersion != "2023-06-01" {
		t.Errorf("unexpected API version: %s", c.APIVersion)
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
	if c.APIVersion == "" {
		t.Error("applyDefaults should set APIVersion")
	}
}
