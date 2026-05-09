package chat

import "github.com/kbukum/gokit/ai"

// FinishReason indicates why model generation stopped.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonToolUse       FinishReason = "tool_use"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonError         FinishReason = "error"
	FinishReasonCancelled     FinishReason = "cancelled" //nolint:misspell // Contract spelling.
)

// MessageStart is emitted when a streaming response begins.
type MessageStart struct {
	ID        string `json:"id"`
	Role      Role   `json:"role,omitempty"`
	Model     string `json:"model"`
	RequestID string `json:"request_id,omitempty"`
}

func (MessageStart) StreamEventMarker() {}

// ToolUseStart is emitted when a tool-use block begins.
type ToolUseStart struct {
	Index int    `json:"index"`
	ID    string `json:"id"`
	Name  string `json:"name"`
}

func (ToolUseStart) StreamEventMarker() {}

// ToolUseDelta carries incremental tool input JSON.
type ToolUseDelta struct {
	Index      int    `json:"index"`
	ID         string `json:"id,omitempty"`
	Name       string `json:"name,omitempty"`
	InputDelta string `json:"input_delta"`
}

func (ToolUseDelta) StreamEventMarker() {}

// ToolUseStop is emitted when a tool-use block completes.
type ToolUseStop struct {
	Index int    `json:"index"`
	ID    string `json:"id,omitempty"`
}

func (ToolUseStop) StreamEventMarker() {}

// ReasoningDelta carries incremental reasoning content.
type ReasoningDelta struct {
	Text string `json:"text"`
}

func (ReasoningDelta) StreamEventMarker() {}

// MessageStop is emitted when the streaming response completes.
type MessageStop struct {
	FinishReason FinishReason `json:"finish_reason"`
	Message      Message      `json:"message,omitempty"`
}

func (MessageStop) StreamEventMarker() {}

var (
	_ ai.StreamEvent = MessageStart{}
	_ ai.StreamEvent = MessageStop{}
	_ ai.StreamEvent = ToolUseStart{}
	_ ai.StreamEvent = ToolUseDelta{}
	_ ai.StreamEvent = ToolUseStop{}
	_ ai.StreamEvent = ReasoningDelta{}
)
