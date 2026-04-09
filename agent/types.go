package agent

import (
	"encoding/json"

	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

// StopReason indicates why the agent loop terminated.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopMaxTurns  StopReason = "max_turns"
	StopMaxBudget StopReason = "max_budget"
	StopAborted   StopReason = "aborted"
)

// Result is the final outcome of an agent run.
type Result struct {
	// Messages is the full conversation history including all turns.
	Messages []llm.Message `json:"messages"`
	// FinalMessage is the last assistant response.
	FinalMessage llm.AssistantMessage `json:"final_message"`
	// TotalUsage is the aggregated token usage across all turns.
	TotalUsage llm.Usage `json:"total_usage"`
	// TurnCount is how many LLM calls were made.
	TurnCount int `json:"turn_count"`
	// StopReason indicates why the loop ended.
	StopReason StopReason `json:"stop_reason"`
}

// Event is the discriminated union for streaming agent events.
type Event interface {
	agentEventMarker()
}

// TurnStartEvent is emitted at the beginning of each agent turn.
type TurnStartEvent struct {
	Turn int `json:"turn"`
}

func (TurnStartEvent) agentEventMarker() {}

// LLMStreamEvent wraps an LLM stream event during streaming.
type LLMStreamEvent struct {
	Event llm.StreamEvent `json:"event"`
}

func (LLMStreamEvent) agentEventMarker() {}

// ToolExecutingEvent is emitted when a tool starts executing.
type ToolExecutingEvent struct {
	ToolUseID string          `json:"tool_use_id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

func (ToolExecutingEvent) agentEventMarker() {}

// ToolCompleteEvent is emitted when a tool finishes executing.
type ToolCompleteEvent struct {
	ToolUseID string       `json:"tool_use_id"`
	Name      string       `json:"name"`
	Result    *tool.Result `json:"result"`
	Err       error        `json:"-"`
}

func (ToolCompleteEvent) agentEventMarker() {}

// ContextCompactedEvent is emitted when messages are compacted.
type ContextCompactedEvent struct {
	OldTokens int `json:"old_tokens"`
	NewTokens int `json:"new_tokens"`
}

func (ContextCompactedEvent) agentEventMarker() {}

// TurnCompleteEvent is emitted at the end of each agent turn.
type TurnCompleteEvent struct {
	Turn    int                  `json:"turn"`
	Message llm.AssistantMessage `json:"message"`
	Usage   llm.Usage            `json:"usage"`
}

func (TurnCompleteEvent) agentEventMarker() {}

// CompleteEvent is emitted when the agent loop finishes.
type CompleteEvent struct {
	Result Result `json:"result"`
}

func (CompleteEvent) agentEventMarker() {}

// ContextStrategy determines how to handle context window overflow.
type ContextStrategy interface {
	// Compact reduces the message list to fit within the context window.
	// It receives the current messages and the max token budget, and returns
	// the compacted messages.
	Compact(messages []llm.Message, maxTokens int) ([]llm.Message, error)
}

// FailStrategy returns an error when the context is exceeded.
type FailStrategy struct{}

func (FailStrategy) Compact(_ []llm.Message, _ int) ([]llm.Message, error) {
	return nil, ErrContextExceeded
}

// TruncateStrategy drops oldest messages, keeping the last N.
type TruncateStrategy struct {
	KeepLast int
}

func (s TruncateStrategy) Compact(messages []llm.Message, _ int) ([]llm.Message, error) {
	if len(messages) <= s.KeepLast {
		return messages, nil
	}
	return messages[len(messages)-s.KeepLast:], nil
}
