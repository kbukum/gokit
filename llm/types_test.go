package llm

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
)

func TestCompletionRequest_Defaults(t *testing.T) {
	req := CompletionRequest{
		Messages: []chat.Message{chat.User("Hello")},
	}

	if req.Model != "" {
		t.Errorf("Model should be empty by default, got %q", req.Model)
	}
	if req.Temperature != nil {
		t.Errorf("Temperature should be nil by default, got %v", req.Temperature)
	}
	if req.Stream {
		t.Error("Stream should be false by default")
	}
}

func TestStreamFormat_Values(t *testing.T) {
	if StreamNDJSON != 0 {
		t.Errorf("StreamNDJSON = %d, want 0", StreamNDJSON)
	}
	if StreamSSE != 1 {
		t.Errorf("StreamSSE = %d, want 1", StreamSSE)
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{Dialect: "test"}
	cfg.applyDefaults()

	if cfg.Timeout != 120e9 { // 120 seconds
		t.Errorf("Timeout = %v, want 120s", cfg.Timeout)
	}
	if cfg.Name != "test-llm" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-llm")
	}
}

func TestConfig_ApplyDefaults_DoesNotOverrideExplicitValues(t *testing.T) {
	cfg := Config{
		Name:    "custom",
		Dialect: "test",
		Timeout: 60e9, // 60 seconds
	}
	cfg.applyDefaults()

	if cfg.Name != "custom" {
		t.Errorf("Name = %q, want %q", cfg.Name, "custom")
	}
	if cfg.Timeout != 60e9 {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
}

func TestUsage_Fields(t *testing.T) {
	u := Usage{
		InputTokens:  10,
		OutputTokens: 20,
	}
	if u.InputTokens+u.OutputTokens != u.TotalTokens() {
		t.Errorf("token math: %d + %d != %d", u.InputTokens, u.OutputTokens, u.TotalTokens())
	}
}

func TestExtra_Field(t *testing.T) {
	req := CompletionRequest{
		Messages: []chat.Message{chat.User("test")},
		Extra:    RawJSON(`{"think":false,"format":"json"}`),
	}

	var fields map[string]any
	if err := json.Unmarshal(req.Extra, &fields); err != nil {
		t.Fatalf("Extra is not a JSON object: %v", err)
	}
	if fields["think"] != false {
		t.Error("Extra['think'] should be false")
	}
	if fields["format"] != "json" {
		t.Errorf("Extra['format'] = %v, want json", fields["format"])
	}
}

func TestCompletionRequest_ZeroTemperature(t *testing.T) {
	zero := 0.0
	req := CompletionRequest{
		Messages:    []chat.Message{chat.User("test")},
		Temperature: &zero,
	}
	if req.Temperature == nil {
		t.Fatal("Temperature should not be nil when set to 0")
	}
	if *req.Temperature != 0.0 {
		t.Errorf("Temperature = %v, want 0.0", *req.Temperature)
	}
}

func TestCompletionRequest_MaxTokensZero(t *testing.T) {
	req := CompletionRequest{
		Messages:  []chat.Message{chat.User("test")},
		MaxTokens: 0,
	}
	if req.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d, want 0 (provider default)", req.MaxTokens)
	}
}

func TestUsage_ZeroValues(t *testing.T) {
	u := Usage{}
	if u.InputTokens != 0 || u.OutputTokens != 0 || u.TotalTokens() != 0 {
		t.Errorf("zero Usage = %+v, want all zeros", u)
	}
}

func TestUsage_JSON_RoundTrip(t *testing.T) {
	u := Usage{InputTokens: 100, OutputTokens: 200}
	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var u2 Usage
	if err := json.Unmarshal(data, &u2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if u2 != u {
		t.Errorf("round-trip = %+v, want %+v", u2, u)
	}
}

func TestStreamChunk_JSON_Serialization(t *testing.T) {
	chunk := streamChunk{Content: "hello", Done: false}
	data, err := json.Marshal(chunk)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var chunk2 streamChunk
	if err := json.Unmarshal(data, &chunk2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if chunk2.Content != "hello" || chunk2.Done != false {
		t.Errorf("round-trip = %+v", chunk2)
	}
}

func TestCompletionResponse_JSON_RoundTrip(t *testing.T) {
	resp := CompletionResponse{
		Message: chat.Assistant("hello world"),
		Model:   "gpt-4",
		Usage:   Usage{InputTokens: 5, OutputTokens: 10},
	}
	if resp.Text() != "hello world" {
		t.Errorf("Text() = %q, want %q", resp.Text(), "hello world")
	}
	if resp.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", resp.Model, "gpt-4")
	}
}

func TestMessageConstructors(t *testing.T) {
	u := chat.User("hello")
	if u.Role() != string(chat.RoleUser) {
		t.Errorf("User.Role() = %q, want %q", u.Role(), chat.RoleUser)
	}
	if ai.TextOf(u.Content) != "hello" {
		t.Errorf("User content = %q, want %q", ai.TextOf(u.Content), "hello")
	}

	a := chat.Assistant("response")
	if a.Role() != string(chat.RoleAssistant) {
		t.Errorf("Assistant.Role() = %q, want %q", a.Role(), chat.RoleAssistant)
	}
	if a.Text() != "response" {
		t.Errorf("Assistant.Text() = %q, want %q", a.Text(), "response")
	}

	s := chat.System("you are helpful")
	if s.Role() != string(chat.RoleSystem) {
		t.Errorf("System.Role() = %q, want %q", s.Role(), chat.RoleSystem)
	}
	if s.Content != "you are helpful" {
		t.Errorf("System.Content = %q, want %q", s.Content, "you are helpful")
	}

	tr := chat.ToolResultMsg("id-1", "result data", false)
	if tr.Role() != string(chat.RoleTool) {
		t.Errorf("ToolResult.Role() = %q, want %q", tr.Role(), chat.RoleTool)
	}
	if tr.ToolUseID != "id-1" {
		t.Errorf("ToolResult.ToolUseID = %q, want %q", tr.ToolUseID, "id-1")
	}
}

func TestMarshalMessage_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		msg  chat.Message
		role string
	}{
		{"user", chat.User("hi"), "user"},
		{"assistant", chat.Assistant("hello"), "assistant"},
		{"system", chat.System("prompt"), "system"},
		{"tool_result", chat.ToolResultMsg("id", "data", false), "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := chat.MarshalMessage(tt.msg)
			if err != nil {
				t.Fatalf("MarshalMessage: %v", err)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if raw["role"] != tt.role {
				t.Errorf("role = %q, want %q", raw["role"], tt.role)
			}
		})
	}
}

func TestStopReason_Values(t *testing.T) {
	tests := []struct {
		sr   chat.FinishReason
		want string
	}{
		{chat.FinishReasonStop, "stop"},
		{chat.FinishReasonToolUse, "tool_use"},
		{chat.FinishReasonLength, "length"},
		{chat.FinishReasonContentFilter, "content_filter"},
	}
	for _, tt := range tests {
		if string(tt.sr) != tt.want {
			t.Errorf("StopReason = %q, want %q", tt.sr, tt.want)
		}
	}
}

func TestContentParts(t *testing.T) {
	tb := ai.Text{Text: "hello"}
	if tb.BlockType() != "text" {
		t.Errorf("ai.Text.BlockType() = %q", tb.BlockType())
	}

	ib := ai.Image{Source: "url", MimeType: "image/png"}
	if ib.BlockType() != "image" {
		t.Errorf("ai.Image.BlockType() = %q", ib.BlockType())
	}

	tub := ai.ToolUseBlock{ID: "1", Name: "test", Input: json.RawMessage(`{}`)}
	if tub.BlockType() != "tool_use" {
		t.Errorf("ai.ToolUseBlock.BlockType() = %q", tub.BlockType())
	}

	trb := ai.ToolResultBlock{ID: "1", Content: "ok"}
	if trb.BlockType() != "tool_result" {
		t.Errorf("ai.ToolResultBlock.BlockType() = %q", trb.BlockType())
	}
}
