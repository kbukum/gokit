package chat

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/ai"
)

var (
	_ Message = UserMessage{}
	_ Message = AssistantMessage{}
	_ Message = SystemMessage{}
	_ Message = ToolResultMessage{}
)

func TestMessageConstructors(t *testing.T) {
	t.Parallel()

	user := User("hello")
	if user.Role() != string(RoleUser) || ai.TextOf(user.Content) != "hello" {
		t.Fatalf("user=%+v", user)
	}

	assistant := Assistant("response")
	if assistant.Role() != string(RoleAssistant) || assistant.Text() != "response" {
		t.Fatalf("assistant=%+v", assistant)
	}

	system := System("you are helpful")
	if system.Role() != string(RoleSystem) || system.Content != "you are helpful" {
		t.Fatalf("system=%+v", system)
	}

	tool := ToolResultMsg("call_1", "ok", true)
	if tool.Role() != string(RoleTool) || tool.ToolUseID != "call_1" || !tool.IsError {
		t.Fatalf("tool=%+v", tool)
	}
}

func TestCountTokensApprox(t *testing.T) {
	t.Parallel()

	msgs := []Message{
		System("You are helpful"),
		User("What is 2+2?"),
		AssistantMessage{ToolCalls: []ai.ToolUseBlock{{ID: "call_1", Name: "calculator", Input: map[string]any{"expr": "2+2"}}}},
		ToolResultMsg("call_1", "4", false),
	}
	if got := CountTokensApprox(msgs); got < 10 {
		t.Fatalf("CountTokensApprox() = %d", got)
	}
}

func TestMarshalMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  Message
		role string
	}{
		{name: "user", msg: User("hi"), role: "user"},
		{name: "assistant", msg: Assistant("hello"), role: "assistant"},
		{name: "system", msg: System("prompt"), role: "system"},
		{name: "tool", msg: ToolResultMsg("id", "data", false), role: "tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := MarshalMessage(tt.msg)
			if err != nil {
				t.Fatalf("MarshalMessage: %v", err)
			}
			var raw map[string]any
			if err := json.Unmarshal(data, &raw); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if raw["role"] != tt.role {
				t.Fatalf("role = %v, want %q", raw["role"], tt.role)
			}
		})
	}
}
