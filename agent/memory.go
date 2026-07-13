package agent

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
)

// Memory provides conversation history persistence for agent sessions.
type Memory interface {
	Load(ctx context.Context, sessionID string) ([]chat.Message, error)
	Save(ctx context.Context, sessionID string, messages []chat.Message) error
	Append(ctx context.Context, sessionID string, messages ...chat.Message) error
	Clear(ctx context.Context, sessionID string) error
}

// InMemoryStore is a thread-safe, in-memory implementation of Memory.
type InMemoryStore struct {
	mu   sync.RWMutex
	data map[string][]chat.Message
}

func NewInMemoryStore() *InMemoryStore { return &InMemoryStore{data: make(map[string][]chat.Message)} }

func (s *InMemoryStore) Load(_ context.Context, sessionID string) ([]chat.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs, ok := s.data[sessionID]
	if !ok {
		return nil, nil
	}
	return copyMessages(msgs), nil
}

func (s *InMemoryStore) Save(_ context.Context, sessionID string, messages []chat.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sessionID] = copyMessages(messages)
	return nil
}

func (s *InMemoryStore) Append(_ context.Context, sessionID string, messages ...chat.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sessionID] = append(s.data[sessionID], copyMessages(messages)...)
	return nil
}

func (s *InMemoryStore) Clear(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, sessionID)
	return nil
}

// SlidingWindowMemory wraps any Memory and keeps only the last N messages.
// If the first message is a SystemMessage, it is preserved outside the window.
type SlidingWindowMemory struct {
	store       Memory
	maxMessages int
}

func NewSlidingWindowMemory(store Memory, maxMessages int) *SlidingWindowMemory {
	return &SlidingWindowMemory{store: store, maxMessages: maxMessages}
}

func (s *SlidingWindowMemory) Load(ctx context.Context, sessionID string) ([]chat.Message, error) {
	msgs, err := s.store.Load(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return s.trimToWindow(msgs), nil
}

func (s *SlidingWindowMemory) Save(ctx context.Context, sessionID string, messages []chat.Message) error {
	return s.store.Save(ctx, sessionID, s.trimToWindow(messages))
}

func (s *SlidingWindowMemory) Append(ctx context.Context, sessionID string, messages ...chat.Message) error {
	return s.store.Append(ctx, sessionID, messages...)
}

func (s *SlidingWindowMemory) Clear(ctx context.Context, sessionID string) error {
	return s.store.Clear(ctx, sessionID)
}

func (s *SlidingWindowMemory) trimToWindow(msgs []chat.Message) []chat.Message {
	if len(msgs) == 0 || s.maxMessages <= 0 {
		return msgs
	}
	var systemMsg chat.Message
	remaining := msgs
	if _, ok := msgs[0].(chat.SystemMessage); ok {
		systemMsg = msgs[0]
		remaining = msgs[1:]
	}
	if len(remaining) > s.maxMessages {
		remaining = remaining[len(remaining)-s.maxMessages:]
	}
	if systemMsg != nil {
		out := make([]chat.Message, 0, 1+len(remaining))
		out = append(out, systemMsg)
		out = append(out, remaining...)
		return out
	}
	return remaining
}

func copyMessages(msgs []chat.Message) []chat.Message {
	out := make([]chat.Message, len(msgs))
	for i, m := range msgs {
		out[i] = copyMessage(m)
	}
	return out
}

func copyMessage(m chat.Message) chat.Message {
	switch msg := m.(type) {
	case chat.UserMessage:
		return chat.UserMessage{Content: copyContentBlocks(msg.Content)}
	case chat.AssistantMessage:
		toolCalls := make([]ai.ToolUseBlock, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			toolCalls[i] = copyToolUseBlock(tc)
		}
		var usage *ai.Usage
		if msg.Usage != nil {
			u := *msg.Usage
			usage = &u
		}
		return chat.AssistantMessage{Content: copyContentBlocks(msg.Content), ToolCalls: toolCalls, Usage: usage}
	case chat.SystemMessage:
		return chat.SystemMessage{Content: msg.Content}
	case chat.ToolResultMessage:
		return chat.ToolResultMessage{ToolUseID: msg.ToolUseID, Content: msg.Content, IsError: msg.IsError}
	default:
		return m
	}
}

func copyContentBlocks(blocks []ai.ContentPart) []ai.ContentPart {
	if blocks == nil {
		return nil
	}
	out := make([]ai.ContentPart, len(blocks))
	for i, b := range blocks {
		switch blk := b.(type) {
		case ai.ToolUseBlock:
			out[i] = copyToolUseBlock(blk)
		default:
			out[i] = blk
		}
	}
	return out
}

func copyToolUseBlock(blk ai.ToolUseBlock) ai.ToolUseBlock {
	if len(blk.Input) == 0 {
		return blk
	}
	cpy := make(json.RawMessage, len(blk.Input))
	copy(cpy, blk.Input)
	blk.Input = cpy
	return blk
}
