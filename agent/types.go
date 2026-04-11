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

// SlidingWindowStrategy keeps a token-budget-aware sliding window.
// It preserves the system message (if present) and removes the oldest
// non-system messages until the total token count fits within the budget.
type SlidingWindowStrategy struct {
	// TokenCounter counts tokens for a message list.
	// When nil, llm.CountTokensApprox is used.
	TokenCounter func([]llm.Message) int
}

func (s SlidingWindowStrategy) Compact(messages []llm.Message, maxTokens int) ([]llm.Message, error) {
	counter := s.TokenCounter
	if counter == nil {
		counter = llm.CountTokensApprox
	}

	if counter(messages) <= maxTokens {
		return messages, nil
	}

	// Separate system message (if first message is system).
	var system llm.Message
	rest := messages
	if len(messages) > 0 {
		if _, ok := messages[0].(llm.SystemMessage); ok {
			system = messages[0]
			rest = messages[1:]
		}
	}

	// Drop oldest messages one at a time until we fit.
	for len(rest) > 1 {
		rest = rest[1:]
		candidate := rest
		if system != nil {
			candidate = append([]llm.Message{system}, rest...)
		}
		if counter(candidate) <= maxTokens {
			return candidate, nil
		}
	}

	// Even a single message doesn't fit — return what we have.
	if system != nil {
		return append([]llm.Message{system}, rest...), nil
	}
	return rest, nil
}

// SummarizeStrategy uses an LLM to summarize dropped messages before
// discarding them. It keeps the last KeepLast messages verbatim and
// replaces earlier messages with a summary.
type SummarizeStrategy struct {
	// Provider is the LLM provider used for summarization.
	Provider llm.Provider
	// KeepLast is the number of recent messages to preserve verbatim.
	KeepLast int
	// SummaryPrompt is the system prompt for the summarization call.
	// Defaults to a reasonable prompt if empty.
	SummaryPrompt string
}

func (s SummarizeStrategy) Compact(messages []llm.Message, _ int) ([]llm.Message, error) {
	keepLast := s.KeepLast
	if keepLast <= 0 {
		keepLast = 4
	}

	if len(messages) <= keepLast {
		return messages, nil
	}

	// Separate system message.
	var system llm.Message
	rest := messages
	if len(messages) > 0 {
		if _, ok := messages[0].(llm.SystemMessage); ok {
			system = messages[0]
			rest = messages[1:]
		}
	}

	if len(rest) <= keepLast {
		if system != nil {
			return append([]llm.Message{system}, rest...), nil
		}
		return rest, nil
	}

	// Split into old (to summarize) and recent (to keep).
	old := rest[:len(rest)-keepLast]
	recent := rest[len(rest)-keepLast:]

	// Build summary of old messages.
	summaryPrompt := s.SummaryPrompt
	if summaryPrompt == "" {
		summaryPrompt = "Summarize this conversation concisely, preserving key facts, decisions, and context needed to continue. Be brief."
	}

	// Format old messages into a single text block.
	var sb llmStrBuilder
	for _, m := range old {
		sb.writeMessage(m)
	}

	if s.Provider != nil {
		resp, err := s.Provider.Complete(llmCtxBackground(), llm.CompletionRequest{
			SystemPrompt: summaryPrompt,
			Messages:     []llm.Message{llm.User(sb.String())},
			MaxTokens:    500,
		})
		if err == nil && resp.Text() != "" {
			summaryMsg := llm.System("[Conversation summary] " + resp.Text())
			result := []llm.Message{summaryMsg}
			if system != nil {
				result = append([]llm.Message{system, summaryMsg}, recent...)
			} else {
				result = append(result, recent...)
			}
			return result, nil
		}
	}

	// Fallback: just truncate.
	if system != nil {
		return append([]llm.Message{system}, recent...), nil
	}
	return recent, nil
}

// llmStrBuilder is a helper for formatting messages as text.
type llmStrBuilder struct {
	buf []byte
}

func (b *llmStrBuilder) writeMessage(m llm.Message) {
	switch msg := m.(type) {
	case llm.UserMessage:
		b.buf = append(b.buf, "User: "...)
		b.buf = append(b.buf, msg.Text()...)
		b.buf = append(b.buf, '\n')
	case llm.AssistantMessage:
		b.buf = append(b.buf, "Assistant: "...)
		b.buf = append(b.buf, msg.Text()...)
		b.buf = append(b.buf, '\n')
	case llm.ToolResultMessage:
		b.buf = append(b.buf, "Tool("...)
		b.buf = append(b.buf, msg.ToolUseID...)
		b.buf = append(b.buf, "): "...)
		b.buf = append(b.buf, msg.Content...)
		b.buf = append(b.buf, '\n')
	}
}

func (b *llmStrBuilder) String() string { return string(b.buf) }

func llmCtxBackground() context.Context {
	return context.Background()
}
