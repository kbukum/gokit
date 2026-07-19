package memory

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
)

// Store provides conversation history persistence for agent sessions.
type Store interface {
	Load(ctx context.Context, sessionID string) ([]chat.Message, error)
	Save(ctx context.Context, sessionID string, messages []chat.Message) error
	Append(ctx context.Context, sessionID string, messages ...chat.Message) error
	Clear(ctx context.Context, sessionID string) error
}

// MapStore is a thread-safe, in-process Store backed by a map.
type MapStore struct {
	mu   sync.RWMutex
	data map[string][]chat.Message
}

// NewMapStore creates an empty in-process Store.
func NewMapStore() *MapStore { return &MapStore{data: make(map[string][]chat.Message)} }

func (s *MapStore) Load(_ context.Context, sessionID string) ([]chat.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs, ok := s.data[sessionID]
	if !ok {
		return nil, nil
	}
	return copyMessages(msgs), nil
}

func (s *MapStore) Save(_ context.Context, sessionID string, messages []chat.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sessionID] = copyMessages(messages)
	return nil
}

func (s *MapStore) Append(_ context.Context, sessionID string, messages ...chat.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sessionID] = append(s.data[sessionID], copyMessages(messages)...)
	return nil
}

func (s *MapStore) Clear(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, sessionID)
	return nil
}

// SlidingWindowStore wraps any Store and keeps only the last N messages.
// If the first message is a SystemMessage, it is preserved outside the window.
type SlidingWindowStore struct {
	store       Store
	maxMessages int
}

// NewSlidingWindowStore wraps store so loads and saves keep at most maxMessages (plus a leading
// system message).
func NewSlidingWindowStore(store Store, maxMessages int) *SlidingWindowStore {
	return &SlidingWindowStore{store: store, maxMessages: maxMessages}
}

func (s *SlidingWindowStore) Load(ctx context.Context, sessionID string) ([]chat.Message, error) {
	msgs, err := s.store.Load(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return s.trimToWindow(msgs), nil
}

func (s *SlidingWindowStore) Save(ctx context.Context, sessionID string, messages []chat.Message) error {
	return s.store.Save(ctx, sessionID, s.trimToWindow(messages))
}

func (s *SlidingWindowStore) Append(ctx context.Context, sessionID string, messages ...chat.Message) error {
	return s.store.Append(ctx, sessionID, messages...)
}

func (s *SlidingWindowStore) Clear(ctx context.Context, sessionID string) error {
	return s.store.Clear(ctx, sessionID)
}

func (s *SlidingWindowStore) trimToWindow(msgs []chat.Message) []chat.Message {
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
