package gemini

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

// ---------------------------------------------------------------------------
// Dialect tests
// ---------------------------------------------------------------------------

func TestDialect_Name(t *testing.T) {
	d := &Dialect{}
	if d.Name() != "gemini" {
		t.Fatalf("expected name 'gemini', got %q", d.Name())
	}
}

func TestDialect_ChatPath(t *testing.T) {
	d := &Dialect{}
	if d.ChatPath() != "/v1beta/models" {
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
		Model:        "gemini-2.0-flash",
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

	// Check system instruction
	si, ok := result["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatal("expected systemInstruction")
	}
	parts := si["parts"].([]any)
	part := parts[0].(map[string]any)
	if part["text"] != "You are helpful." {
		t.Errorf("expected system instruction text, got %v", part["text"])
	}

	// Check contents
	contents := result["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content entry, got %d", len(contents))
	}
	content := contents[0].(map[string]any)
	if content["role"] != "user" {
		t.Errorf("expected role 'user', got %v", content["role"])
	}
}

func TestDialect_BuildRequest_WithGenerationConfig(t *testing.T) {
	d := &Dialect{}
	temp := 0.7
	topP := 0.9
	req := llm.CompletionRequest{
		Model:       "gemini-2.0-flash",
		Messages:    []llm.Message{llm.User("test")},
		Temperature: &temp,
		TopP:        &topP,
		MaxTokens:   1000,
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	gc, ok := result["generationConfig"].(map[string]any)
	if !ok {
		t.Fatal("expected generationConfig")
	}
	if gc["temperature"].(float64) != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", gc["temperature"])
	}
	if gc["topP"].(float64) != 0.9 {
		t.Errorf("expected topP 0.9, got %v", gc["topP"])
	}
	if int(gc["maxOutputTokens"].(float64)) != 1000 {
		t.Errorf("expected maxOutputTokens 1000, got %v", gc["maxOutputTokens"])
	}
}

func TestDialect_BuildRequest_WithTools(t *testing.T) {
	d := &Dialect{}
	req := llm.CompletionRequest{
		Model:    "gemini-2.0-flash",
		Messages: []llm.Message{llm.User("What's the weather?")},
		Tools: []tool.Definition{
			{
				Name:        "get_weather",
				Description: "Get weather for a city",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	tools := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool group, got %d", len(tools))
	}
	toolGroup := tools[0].(map[string]any)
	decls := toolGroup["functionDeclarations"].([]any)
	if len(decls) != 1 {
		t.Fatalf("expected 1 function declaration, got %d", len(decls))
	}
	decl := decls[0].(map[string]any)
	if decl["name"] != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %v", decl["name"])
	}
}

func TestDialect_BuildRequest_SystemMessageSkipped(t *testing.T) {
	d := &Dialect{}
	req := llm.CompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []llm.Message{
			llm.System("system msg"),
			llm.User("hello"),
		},
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	// System messages are skipped in contents (handled via systemInstruction)
	contents := result["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content (system skipped), got %d", len(contents))
	}
}

func TestDialect_ParseResponse_Text(t *testing.T) {
	d := &Dialect{}

	raw := `{
		"candidates": [{
			"content": {
				"parts": [{"text": "Hello!"}],
				"role": "model"
			},
			"finishReason": "STOP"
		}],
		"usageMetadata": {
			"promptTokenCount": 10,
			"candidatesTokenCount": 5,
			"totalTokenCount": 15
		},
		"modelVersion": "gemini-2.0-flash"
	}`

	resp, err := d.ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}

	if resp.Model != "gemini-2.0-flash" {
		t.Errorf("expected model gemini-2.0-flash, got %s", resp.Model)
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

func TestDialect_ParseResponse_FunctionCall(t *testing.T) {
	d := &Dialect{}

	raw := `{
		"candidates": [{
			"content": {
				"parts": [
					{"text": "Let me check the weather."},
					{"functionCall": {"name": "get_weather", "args": {"city": "NYC"}}}
				],
				"role": "model"
			},
			"finishReason": "TOOL_USE"
		}],
		"usageMetadata": {"promptTokenCount": 20, "candidatesTokenCount": 10, "totalTokenCount": 30}
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

func TestDialect_ParseResponse_NoCandidates(t *testing.T) {
	d := &Dialect{}
	_, err := d.ParseResponse([]byte(`{"candidates":[]}`))
	if err == nil {
		t.Fatal("expected error for empty candidates")
	}
}

func TestDialect_ParseResponse_SafetyFilter(t *testing.T) {
	d := &Dialect{}

	raw := `{
		"candidates": [{
			"content": {"parts": [{"text": "I cannot help with that."}], "role": "model"},
			"finishReason": "SAFETY"
		}],
		"usageMetadata": {"promptTokenCount": 5, "candidatesTokenCount": 8, "totalTokenCount": 13}
	}`

	resp, err := d.ParseResponse([]byte(raw))
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	if resp.StopReason != llm.StopContentFilter {
		t.Errorf("expected StopContentFilter for SAFETY, got %v", resp.StopReason)
	}
}

func TestDialect_ParseStreamChunk_Content(t *testing.T) {
	d := &Dialect{}

	data := `{"candidates":[{"content":{"parts":[{"text":"Hello"}]},"finishReason":""}]}`
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

	data := `{"candidates":[{"content":{"parts":[{"text":""}]},"finishReason":"STOP"}]}`
	chunk, err := d.ParseStreamChunk([]byte(data))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if !chunk.Done {
		t.Error("expected done=true for finishReason STOP")
	}
}

func TestDialect_ParseStreamChunk_Empty(t *testing.T) {
	d := &Dialect{}

	data := `{"candidates":[]}`
	chunk, err := d.ParseStreamChunk([]byte(data))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if chunk.Content != "" {
		t.Errorf("expected empty content, got %q", chunk.Content)
	}
	if chunk.Done {
		t.Error("expected done=false for empty candidates")
	}
}

func TestDialect_ParseStreamChunk_FunctionCall(t *testing.T) {
	d := &Dialect{}

	data := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"search","args":{"q":"test"}}}]},"finishReason":""}]}`
	chunk, err := d.ParseStreamChunk([]byte(data))
	if err != nil {
		t.Fatalf("ParseStreamChunk: %v", err)
	}
	if len(chunk.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(chunk.ToolCalls))
	}
	if chunk.ToolCalls[0].Function.Name != "search" {
		t.Errorf("tool name = %q, want %q", chunk.ToolCalls[0].Function.Name, "search")
	}
}

func TestDialect_RegisteredViaInit(t *testing.T) {
	d, err := llm.GetDialect("gemini")
	if err != nil {
		t.Fatalf("GetDialect(gemini): %v", err)
	}
	if d.Name() != "gemini" {
		t.Errorf("expected gemini dialect, got %q", d.Name())
	}
}

func TestDialect_ToolResultEncoding(t *testing.T) {
	d := &Dialect{}
	req := llm.CompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []llm.Message{
			llm.User("what's the weather?"),
			llm.ToolResultMsg("get_weather", "72°F and sunny", false),
		},
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	contents := result["contents"].([]any)
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(contents))
	}

	toolResult := contents[1].(map[string]any)
	if toolResult["role"] != "user" {
		t.Errorf("expected role 'user' for tool result, got %v", toolResult["role"])
	}

	parts := toolResult["parts"].([]any)
	part := parts[0].(map[string]any)
	fr, ok := part["functionResponse"].(map[string]any)
	if !ok {
		t.Fatal("expected functionResponse in tool result part")
	}
	if fr["name"] != "get_weather" {
		t.Errorf("expected function name 'get_weather', got %v", fr["name"])
	}
}

func TestDialect_AssistantMessageEncoding(t *testing.T) {
	d := &Dialect{}
	req := llm.CompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []llm.Message{
			llm.User("hello"),
			llm.Assistant("Hi there!"),
		},
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}

	bs, _ := json.Marshal(body)
	var result map[string]any
	_ = json.Unmarshal(bs, &result)

	contents := result["contents"].([]any)
	if len(contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(contents))
	}

	assistant := contents[1].(map[string]any)
	if assistant["role"] != "model" {
		t.Errorf("expected role 'model' for assistant, got %v", assistant["role"])
	}
}

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.BaseURL != "https://generativelanguage.googleapis.com" {
		t.Errorf("unexpected base URL: %s", c.BaseURL)
	}
	if c.Model != "gemini-2.0-flash" {
		t.Errorf("unexpected model: %s", c.Model)
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	c := Config{APIKey: "test-key"}
	c.applyDefaults()
	if c.BaseURL == "" {
		t.Error("applyDefaults should set BaseURL")
	}
	if c.Model == "" {
		t.Error("applyDefaults should set Model")
	}
}
