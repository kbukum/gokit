package chat_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
)

func TestStreamEventsAndErrors(t *testing.T) {
	t.Parallel()

	events := []ai.StreamEvent{
		chat.MessageStart{ID: "m", Role: chat.RoleAssistant, Model: "model", RequestID: "r"},
		ai.TextDelta{Index: 0, Text: "hi"},
		chat.ReasoningDelta{Text: "because"},
		chat.ToolUseStart{Index: 1, ID: "call", Name: "tool"},
		chat.ToolUseDelta{Index: 1, InputDelta: "{}"},
		chat.ToolUseStop{Index: 1, ID: "call"},
		ai.UsageDelta{InputTokens: 1, OutputTokens: 2, CachedTokens: 1, ReasoningTokens: 1},
		chat.MessageStop{FinishReason: chat.FinishReasonToolUse},
		ai.Error{Err: context.Canceled},
	}
	if len(events) != 9 {
		t.Fatalf("events = %d", len(events))
	}
	streamErr, ok := events[8].(ai.Error)
	if !ok || !errors.Is(streamErr, context.Canceled) || streamErr.Error() != context.Canceled.Error() {
		t.Fatalf("stream error unwrap failed: %#v", events[8])
	}
}

// TestChatStreamEventTypeWireStrings locks the canonical event-type wire
// strings for chat-layer stream events.
func TestChatStreamEventTypeWireStrings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		event ai.StreamEvent
		want  string
	}{
		{chat.MessageStart{}, "message.start"},
		{chat.ReasoningDelta{}, "reasoning.delta"},
		{chat.ToolUseStart{}, "tool_use.start"},
		{chat.ToolUseDelta{}, "tool_use.delta"},
		{chat.ToolUseStop{}, "tool_use.stop"},
		{chat.MessageStop{}, "message.stop"},
	}
	for _, tc := range cases {
		if got := tc.event.EventType(); got != tc.want {
			t.Errorf("%T.EventType() = %q, want %q", tc.event, got, tc.want)
		}
	}
}
