package llm

import (
	"context"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
)

// StreamEvent is the canonical AI streaming event sum-type.
type StreamEvent = ai.StreamEvent

// Canonical event aliases (D5). These mirror the AI/chat names exactly so
// downstream consumers can refer to the llm-package names without going
// through ai.* / chat.*. Redundant aliases (ContentDelta, UsageUpdate,
// ToolCallDelta) have been removed per D5.
type (
	MessageStart   = chat.MessageStart
	TextDelta      = ai.TextDelta
	ReasoningDelta = chat.ReasoningDelta
	ToolUseStart   = chat.ToolUseStart
	ToolUseDelta   = chat.ToolUseDelta
	ToolUseStop    = chat.ToolUseStop
	UsageDelta     = ai.UsageDelta
	StreamError    = ai.Error
	Error          = ai.Error
)

// MessageComplete is the llm-layer terminal event carrying the assembled
// CompletionResponse alongside the canonical chat.MessageStop signal.
type MessageComplete struct {
	Response CompletionResponse `json:"response"`
}

func (MessageComplete) StreamEventMarker() {}

type MessageStop = chat.MessageStop

var ErrCancelled = context.Canceled
