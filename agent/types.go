package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

// StopReason indicates why the agent loop terminated. It is a type alias for chat.FinishReason so LLM-sourced stops require no conversion — the LLM's finish reason IS the stop reason. Agent-policy stops (max turns, wall clock, etc.) extend the value set with strings that have no chat equivalent.
type StopReason = chat.FinishReason

const (
	StopEndTurn   StopReason = chat.FinishReasonStop
	StopMaxTokens StopReason = chat.FinishReasonLength
	StopCancelled StopReason = chat.FinishReasonCancelled
	StopError     StopReason = chat.FinishReasonError

	StopMaxTurns     StopReason = "max_turns"
	StopMaxToolCalls StopReason = "max_tool_calls"
	StopWallClock    StopReason = "wall_clock"
	StopCommand      StopReason = "command"
)

var (
	ErrCancelled            = errors.New("agent: canceled")
	ErrWallClockExceeded    = ai.BudgetExceededError{Reason: ai.BudgetExceededWallClock}
	ErrMaxToolCallsExceeded = ai.BudgetExceededError{Reason: ai.BudgetExceededCalls}
	ErrMaxTokensExceeded    = ai.BudgetExceededError{Reason: ai.BudgetExceededTokens}
	ErrMaxTurnsExceeded     = errors.New("agent: max turns exceeded")
)

// Result is the final outcome of an agent run.
type Result struct {
	Messages     []chat.Message        `json:"messages"`
	FinalMessage chat.AssistantMessage `json:"final_message"`
	TotalUsage   llm.Usage             `json:"total_usage"`
	TurnCount    int                   `json:"turn_count"`
	StopReason   StopReason            `json:"stop_reason"`
}

// MemoryPolicy determines how to handle context window overflow.
type MemoryPolicy interface {
	Compact(ctx context.Context, messages []chat.Message, maxTokens int) ([]chat.Message, error)
}

type RingBufferPolicy struct{ KeepLast int }

func (p RingBufferPolicy) Compact(_ context.Context, messages []chat.Message, _ int) ([]chat.Message, error) {
	keep := p.KeepLast
	if keep <= 0 {
		keep = 20
	}
	if len(messages) <= keep {
		return messages, nil
	}
	var system chat.Message
	rest := messages
	if len(messages) > 0 {
		if _, ok := messages[0].(chat.SystemMessage); ok {
			system = messages[0]
			rest = messages[1:]
		}
	}
	if len(rest) > keep {
		rest = rest[len(rest)-keep:]
	}
	if system != nil {
		return append([]chat.Message{system}, rest...), nil
	}
	return rest, nil
}

type FailStrategy struct{}

func (FailStrategy) Compact(_ context.Context, _ []chat.Message, _ int) ([]chat.Message, error) {
	return nil, ErrContextExceeded
}

type TruncateStrategy struct{ KeepLast int }

func (s TruncateStrategy) Compact(_ context.Context, messages []chat.Message, _ int) ([]chat.Message, error) {
	if len(messages) <= s.KeepLast {
		return messages, nil
	}
	return messages[len(messages)-s.KeepLast:], nil
}

type SlidingWindowStrategy struct{ TokenCounter func([]chat.Message) int }

func (s SlidingWindowStrategy) Compact(_ context.Context, messages []chat.Message, maxTokens int) ([]chat.Message, error) {
	counter := s.TokenCounter
	if counter == nil {
		counter = chat.CountTokensApprox
	}
	if counter(messages) <= maxTokens {
		return messages, nil
	}
	var system chat.Message
	rest := messages
	if len(messages) > 0 {
		if _, ok := messages[0].(chat.SystemMessage); ok {
			system = messages[0]
			rest = messages[1:]
		}
	}
	for len(rest) > 1 {
		rest = rest[1:]
		candidate := rest
		if system != nil {
			candidate = append([]chat.Message{system}, rest...)
		}
		if counter(candidate) <= maxTokens {
			return candidate, nil
		}
	}
	if system != nil {
		return append([]chat.Message{system}, rest...), nil
	}
	return rest, nil
}

type SummarizeStrategy struct {
	Provider      llm.Provider
	KeepLast      int
	SummaryPrompt string
}

func (s SummarizeStrategy) Compact(ctx context.Context, messages []chat.Message, _ int) ([]chat.Message, error) {
	keepLast := s.KeepLast
	if keepLast <= 0 {
		keepLast = 4
	}
	if len(messages) <= keepLast {
		return messages, nil
	}

	var system chat.Message
	rest := messages
	if len(messages) > 0 {
		if _, ok := messages[0].(chat.SystemMessage); ok {
			system = messages[0]
			rest = messages[1:]
		}
	}
	if len(rest) <= keepLast {
		if system != nil {
			return append([]chat.Message{system}, rest...), nil
		}
		return rest, nil
	}

	old := rest[:len(rest)-keepLast]
	recent := rest[len(rest)-keepLast:]
	summaryPrompt := s.SummaryPrompt
	if summaryPrompt == "" {
		summaryPrompt = "Summarize this conversation concisely, preserving key facts, decisions, and context needed to continue. Be brief."
	}

	var sb strings.Builder
	for _, m := range old {
		switch msg := m.(type) {
		case chat.UserMessage:
			sb.WriteString("User: ")
			sb.WriteString(ai.TextOf(msg.Content))
			sb.WriteByte('\n')
		case chat.AssistantMessage:
			sb.WriteString("Assistant: ")
			sb.WriteString(msg.Text())
			sb.WriteByte('\n')
		case chat.ToolResultMessage:
			sb.WriteString("Tool(")
			sb.WriteString(msg.ToolUseID)
			sb.WriteString("): ")
			sb.WriteString(msg.Content)
			sb.WriteByte('\n')
		}
	}

	if s.Provider != nil {
		resp, err := s.Provider.Execute(ctx, llm.CompletionRequest{SystemPrompt: summaryPrompt, Messages: []chat.Message{chat.User(sb.String())}, MaxTokens: 500})
		if err == nil && resp.Text() != "" {
			summaryMsg := chat.System("[Conversation summary] " + resp.Text())
			if system != nil {
				return append([]chat.Message{system, summaryMsg}, recent...), nil
			}
			return append([]chat.Message{summaryMsg}, recent...), nil
		}
	}
	if system != nil {
		return append([]chat.Message{system}, recent...), nil
	}
	return recent, nil
}
