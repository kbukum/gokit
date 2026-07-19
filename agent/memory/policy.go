package memory

import (
	"context"
	"errors"
	"strings"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

// ErrContextExceeded is returned by a Policy that refuses to compact an over-budget conversation.
var ErrContextExceeded = errors.New("agent: context window exceeded")

// Policy determines how to handle context window overflow by compacting a message slice.
type Policy interface {
	Compact(ctx context.Context, messages []chat.Message, maxTokens int) ([]chat.Message, error)
}

// RingBuffer keeps only the last KeepLast messages, preserving a leading system message.
type RingBuffer struct{ KeepLast int }

func (p RingBuffer) Compact(_ context.Context, messages []chat.Message, _ int) ([]chat.Message, error) {
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

// Fail refuses compaction, returning ErrContextExceeded.
type Fail struct{}

func (Fail) Compact(_ context.Context, _ []chat.Message, _ int) ([]chat.Message, error) {
	return nil, ErrContextExceeded
}

// Truncate keeps only the last KeepLast messages, without preserving a leading system message.
type Truncate struct{ KeepLast int }

func (s Truncate) Compact(_ context.Context, messages []chat.Message, _ int) ([]chat.Message, error) {
	if len(messages) <= s.KeepLast {
		return messages, nil
	}
	return messages[len(messages)-s.KeepLast:], nil
}

// SlidingWindow drops oldest messages until the token count fits maxTokens, preserving a leading system message.
type SlidingWindow struct{ TokenCounter func([]chat.Message) int }

func (s SlidingWindow) Compact(_ context.Context, messages []chat.Message, maxTokens int) ([]chat.Message, error) {
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

// Summarize replaces the older messages with an LLM-generated summary, keeping the last KeepLast verbatim.
type Summarize struct {
	Provider      llm.Provider
	KeepLast      int
	SummaryPrompt string
}

func (s Summarize) Compact(ctx context.Context, messages []chat.Message, _ int) ([]chat.Message, error) {
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
