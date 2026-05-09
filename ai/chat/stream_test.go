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
