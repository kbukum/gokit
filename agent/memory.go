package agent

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/kbukum/gokit/llm"
)

// Memory provides conversation history persistence for agent sessions.
type Memory interface {
	// Load retrieves conversation history for a session.
	Load(ctx context.Context, sessionID string) ([]llm.Message, error)
	// Save persists the conversation history for a session.
	Save(ctx context.Context, sessionID string, messages []llm.Message) error
	// Append adds messages to the existing history without loading the full set.
	Append(ctx context.Context, sessionID string, messages ...llm.Message) error
	// Clear deletes all messages for a session.
	Clear(ctx context.Context, sessionID string) error
}

// --- InMemoryStore ---

// InMemoryStore is a thread-safe, in-memory implementation of Memory.
type InMemoryStore struct {
	mu   sync.RWMutex
	data map[string][]llm.Message
}

// NewInMemoryStore creates a new InMemoryStore.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{data: make(map[string][]llm.Message)}
}

func (s *InMemoryStore) Load(_ context.Context, sessionID string) ([]llm.Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	msgs, ok := s.data[sessionID]
	if !ok {
		return nil, nil
	}
	return copyMessages(msgs), nil
}

func (s *InMemoryStore) Save(_ context.Context, sessionID string, messages []llm.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[sessionID] = copyMessages(messages)
	return nil
}

func (s *InMemoryStore) Append(_ context.Context, sessionID string, messages ...llm.Message) error {
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

// --- SlidingWindowMemory ---

// SlidingWindowMemory wraps any Memory and keeps only the last N messages.
// If the first message is a SystemMessage, it is preserved outside the window.
type SlidingWindowMemory struct {
	store       Memory
	maxMessages int
}

// NewSlidingWindowMemory creates a SlidingWindowMemory that wraps store
// and retains at most maxMessages non-system messages.
func NewSlidingWindowMemory(store Memory, maxMessages int) *SlidingWindowMemory {
	return &SlidingWindowMemory{store: store, maxMessages: maxMessages}
}

func (s *SlidingWindowMemory) Load(ctx context.Context, sessionID string) ([]llm.Message, error) {
	msgs, err := s.store.Load(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return s.trimToWindow(msgs), nil
}

func (s *SlidingWindowMemory) Save(ctx context.Context, sessionID string, messages []llm.Message) error {
	return s.store.Save(ctx, sessionID, s.trimToWindow(messages))
}

func (s *SlidingWindowMemory) Append(ctx context.Context, sessionID string, messages ...llm.Message) error {
	return s.store.Append(ctx, sessionID, messages...)
}

func (s *SlidingWindowMemory) Clear(ctx context.Context, sessionID string) error {
	return s.store.Clear(ctx, sessionID)
}

func (s *SlidingWindowMemory) trimToWindow(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 || s.maxMessages <= 0 {
		return msgs
	}

	var systemMsg llm.Message
	remaining := msgs
	if _, ok := msgs[0].(llm.SystemMessage); ok {
		systemMsg = msgs[0]
		remaining = msgs[1:]
	}

	if len(remaining) > s.maxMessages {
		remaining = remaining[len(remaining)-s.maxMessages:]
	}

	if systemMsg != nil {
		out := make([]llm.Message, 0, 1+len(remaining))
		out = append(out, systemMsg)
		out = append(out, remaining...)
		return out
	}
	return remaining
}

// --- deep copy helpers ---

func copyMessages(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		out[i] = copyMessage(m)
	}
	return out
}

func copyMessage(m llm.Message) llm.Message {
	switch msg := m.(type) {
	case llm.UserMessage:
		return llm.UserMessage{Content: copyContentBlocks(msg.Content)}
	case llm.AssistantMessage:
		tc := make([]llm.ToolCall, len(msg.ToolCalls))
		copy(tc, msg.ToolCalls)
		var usage *llm.Usage
		if msg.Usage != nil {
			u := *msg.Usage
			usage = &u
		}
		return llm.AssistantMessage{
			Content:   copyContentBlocks(msg.Content),
			ToolCalls: tc,
			Usage:     usage,
		}
	case llm.SystemMessage:
		return llm.SystemMessage{Content: msg.Content}
	case llm.ToolResultMessage:
		return llm.ToolResultMessage{
			ToolUseID: msg.ToolUseID,
			Content:   msg.Content,
			IsError:   msg.IsError,
		}
	default:
		return m
	}
}

func copyContentBlocks(blocks []llm.ContentBlock) []llm.ContentBlock {
	if blocks == nil {
		return nil
	}
	out := make([]llm.ContentBlock, len(blocks))
	for i, b := range blocks {
		switch blk := b.(type) {
		case llm.ToolUseBlock:
			input := make(json.RawMessage, len(blk.Input))
			copy(input, blk.Input)
			out[i] = llm.ToolUseBlock{ID: blk.ID, Name: blk.Name, Input: input}
		default:
			out[i] = blk
		}
	}
	return out
}
