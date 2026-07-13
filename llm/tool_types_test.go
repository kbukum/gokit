package llm

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/ai/chat"

	"github.com/kbukum/gokit/ai"
)

func TestStreamToolCallSerialization(t *testing.T) {
	tc := streamToolCall{Index: 1, ID: "call_abc123", Name: "get_weather", InputDelta: `{"location":"San Francisco"}`}
	data, err := json.Marshal(tc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got streamToolCall
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != tc.ID || got.Name != tc.Name || got.InputDelta != tc.InputDelta || got.Index != tc.Index {
		t.Fatalf("got %+v want %+v", got, tc)
	}
}

func TestToolResultToMessage(t *testing.T) {
	result := ToolResult{ToolCallID: "call_abc123", Content: `{"temperature":22}`, IsError: false}
	msg := result.ToMessage()
	if msg.Role() != string(chat.RoleTool) {
		t.Fatalf("Role() = %q", msg.Role())
	}
	if msg.Content != result.Content || msg.ToolUseID != result.ToolCallID {
		t.Fatalf("msg=%+v result=%+v", msg, result)
	}
}

func TestToolChoiceSerialization(t *testing.T) {
	tests := []struct {
		name string
		tc   *ToolChoice
		want string
	}{
		{name: "auto", tc: ToolChoiceAuto, want: `{"mode":"auto"}`},
		{name: "none", tc: ToolChoiceNone, want: `{"mode":"none"}`},
		{name: "required", tc: ToolChoiceRequired, want: `{"mode":"required"}`},
		{name: "specific", tc: ToolChoiceFunc("get_weather"), want: `{"mode":"specific","function":"get_weather"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.tc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != tt.want {
				t.Fatalf("got %s want %s", data, tt.want)
			}
		})
	}
}

func TestCompletionResponseHasToolCalls(t *testing.T) {
	resp := CompletionResponse{Message: chat.Assistant("hello")}
	if resp.HasToolCalls() {
		t.Fatal("empty response should not have tool calls")
	}
	resp.Message.ToolCalls = []ai.ToolUseBlock{{ID: "1", Name: "test", Input: json.RawMessage(`{}`)}}
	if !resp.HasToolCalls() {
		t.Fatal("response with tool calls should return true")
	}
}

func TestAssistantMessageWithToolCalls(t *testing.T) {
	msg := chat.AssistantMessage{ToolCalls: []ai.ToolUseBlock{{ID: "call_1", Name: "search", Input: json.RawMessage(`{"q":"test"}`)}, {ID: "call_2", Name: "fetch", Input: json.RawMessage(`{"url":"https://example.com"}`)}}}
	if !msg.HasToolCalls() || len(msg.ToolCalls) != 2 {
		t.Fatalf("msg=%+v", msg)
	}
	if msg.ToolCalls[0].Name != "search" || msg.ToolCalls[1].Name != "fetch" {
		t.Fatalf("unexpected tool calls: %+v", msg.ToolCalls)
	}
}

func TestMarshalMessageOmitsEmpty(t *testing.T) {
	data, err := chat.MarshalMessage(chat.User("hello"))
	if err != nil {
		t.Fatalf("MarshalMessage: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected serialized message")
	}
}

func TestCompletionRequestWithToolChoice(t *testing.T) {
	req := CompletionRequest{Model: "gpt-4", Messages: []chat.Message{chat.User("What's the weather?")}, ToolChoice: ToolChoiceAuto}
	if req.Model != "gpt-4" || req.ToolChoice == nil || req.ToolChoice.Mode != "auto" {
		t.Fatalf("req=%+v", req)
	}
}
