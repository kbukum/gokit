package llm

import (
	"encoding/json"
	"testing"
)

func TestToolCallSerialization(t *testing.T) {
	tc := ToolCall{
		ID:   "call_abc123",
		Type: "function",
		Function: FunctionCall{
			Name:      "get_weather",
			Arguments: `{"location":"San Francisco","unit":"celsius"}`,
		},
	}

	data, err := json.Marshal(tc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ToolCall
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != tc.ID {
		t.Errorf("ID = %q, want %q", got.ID, tc.ID)
	}
	if got.Type != tc.Type {
		t.Errorf("Type = %q, want %q", got.Type, tc.Type)
	}
	if got.Function.Name != tc.Function.Name {
		t.Errorf("Function.Name = %q, want %q", got.Function.Name, tc.Function.Name)
	}
	if got.Function.Arguments != tc.Function.Arguments {
		t.Errorf("Function.Arguments = %q, want %q", got.Function.Arguments, tc.Function.Arguments)
	}
}

func TestToolResultToMessage(t *testing.T) {
	result := ToolResult{
		ToolCallID: "call_abc123",
		Content:    `{"temperature":22,"unit":"celsius"}`,
		IsError:    false,
	}

	msg := result.ToMessage()

	if msg.Role() != RoleTool {
		t.Errorf("Role() = %q, want %q", msg.Role(), RoleTool)
	}
	if msg.Content != result.Content {
		t.Errorf("Content = %q, want %q", msg.Content, result.Content)
	}
	if msg.ToolUseID != result.ToolCallID {
		t.Errorf("ToolUseID = %q, want %q", msg.ToolUseID, result.ToolCallID)
	}
}

func TestToolResultToMessageError(t *testing.T) {
	result := ToolResult{
		ToolCallID: "call_err",
		Content:    "tool not found",
		IsError:    true,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ToolResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.IsError != true {
		t.Error("IsError should be true")
	}
}

func TestToolChoiceSerialization(t *testing.T) {
	tests := []struct {
		name string
		tc   *ToolChoice
		want string
	}{
		{
			name: "auto",
			tc:   ToolChoiceAuto,
			want: `{"mode":"auto"}`,
		},
		{
			name: "none",
			tc:   ToolChoiceNone,
			want: `{"mode":"none"}`,
		},
		{
			name: "required",
			tc:   ToolChoiceRequired,
			want: `{"mode":"required"}`,
		},
		{
			name: "specific",
			tc:   ToolChoiceFunc("get_weather"),
			want: `{"mode":"specific","function":"get_weather"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.tc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("got %s, want %s", data, tt.want)
			}

			var got ToolChoice
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if got.Mode != tt.tc.Mode {
				t.Errorf("Mode = %q, want %q", got.Mode, tt.tc.Mode)
			}
			if got.Function != tt.tc.Function {
				t.Errorf("Function = %q, want %q", got.Function, tt.tc.Function)
			}
		})
	}
}

func TestCompletionResponseHasToolCalls(t *testing.T) {
	resp := CompletionResponse{Message: Assistant("hello")}
	if resp.HasToolCalls() {
		t.Error("empty response should not have tool calls")
	}

	resp.Message.ToolCalls = []ToolCall{
		{ID: "1", Type: "function", Function: FunctionCall{Name: "test", Arguments: "{}"}},
	}
	if !resp.HasToolCalls() {
		t.Error("response with tool calls should return true")
	}
}

func TestAssistantMessageWithToolCalls(t *testing.T) {
	msg := AssistantMessage{
		ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "search", Arguments: `{"q":"test"}`}},
			{ID: "call_2", Type: "function", Function: FunctionCall{Name: "fetch", Arguments: `{"url":"https://example.com"}`}},
		},
	}

	if !msg.HasToolCalls() {
		t.Error("expected HasToolCalls() = true")
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "search" {
		t.Errorf("first tool call name = %q, want %q", msg.ToolCalls[0].Function.Name, "search")
	}
	if msg.ToolCalls[1].Function.Name != "fetch" {
		t.Errorf("second tool call name = %q, want %q", msg.ToolCalls[1].Function.Name, "fetch")
	}
}

func TestMarshalMessage_OmitsEmpty(t *testing.T) {
	data, err := MarshalMessage(User("hello"))
	if err != nil {
		t.Fatalf("MarshalMessage: %v", err)
	}

	str := string(data)
	if contains(str, "tool_calls") {
		t.Error("tool_calls should be omitted for user message")
	}
}

func TestRoleConstants(t *testing.T) {
	if RoleSystem != "system" {
		t.Errorf("RoleSystem = %q", RoleSystem)
	}
	if RoleUser != "user" {
		t.Errorf("RoleUser = %q", RoleUser)
	}
	if RoleAssistant != "assistant" {
		t.Errorf("RoleAssistant = %q", RoleAssistant)
	}
	if RoleTool != "tool" {
		t.Errorf("RoleTool = %q", RoleTool)
	}
}

func TestCompletionRequestWithToolChoice(t *testing.T) {
	req := CompletionRequest{
		Model:      "gpt-4",
		Messages:   []Message{User("What's the weather?")},
		ToolChoice: ToolChoiceAuto,
	}

	if req.Model != "gpt-4" {
		t.Errorf("Model = %q", req.Model)
	}
	if req.ToolChoice == nil {
		t.Fatal("ToolChoice should not be nil")
	}
	if req.ToolChoice.Mode != "auto" {
		t.Errorf("ToolChoice.Mode = %q, want %q", req.ToolChoice.Mode, "auto")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
