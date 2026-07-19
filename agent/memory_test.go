package agent_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/agent"
	"github.com/kbukum/gokit/agent/memory"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

// --- Integration: Agent Run with Memory ---

func TestAgent_RunWithMemory(t *testing.T) {
	store := memory.NewMapStore()

	// Mock provider that echoes the number of input messages
	provider := &echoCountProvider{}

	a := agent.New(agent.Config{
		Provider:  provider,
		Store:     store,
		SessionID: "test-session",
	})
	ctx := context.Background()

	// First run: single user message
	result1, err := a.Run(ctx, []chat.Message{chat.User("hello")})
	if err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if result1.FinalMessage.Text() != "seen 1 messages" {
		t.Errorf("run 1: got %q, want %q", result1.FinalMessage.Text(), "seen 1 messages")
	}

	// Second run: history (user+assistant) should be loaded, plus new user msg = 3
	result2, err := a.Run(ctx, []chat.Message{chat.User("world")})
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if result2.FinalMessage.Text() != "seen 3 messages" {
		t.Errorf("run 2: got %q, want %q", result2.FinalMessage.Text(), "seen 3 messages")
	}

	// Verify memory was saved with full history (4 messages after run 2)
	saved, _ := store.Load(ctx, "test-session")
	if len(saved) != 4 {
		t.Errorf("expected 4 saved messages, got %d", len(saved))
	}
}

func TestAgent_RunWithSlidingWindowStore(t *testing.T) {
	store := memory.NewMapStore()
	sw := memory.NewSlidingWindowStore(store, 4)

	provider := &echoCountProvider{}

	a := agent.New(agent.Config{
		Provider:  provider,
		Store:     sw,
		SessionID: "windowed",
	})
	ctx := context.Background()

	// Run several conversations to accumulate history
	for i := 0; i < 5; i++ {
		_, err := a.Run(ctx, []chat.Message{chat.User(fmt.Sprintf("msg-%d", i))})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}

	// The sliding window should only keep 4 messages in storage
	raw, _ := store.Load(ctx, "windowed")
	if len(raw) > 4 {
		t.Errorf("expected at most 4 stored messages, got %d", len(raw))
	}
}

func TestAgent_RunWithoutMemory(t *testing.T) {
	// Ensure existing behavior is unaffected when Memory is nil
	provider := newMockProvider(textResponse("ok"))
	a := agent.New(agent.Config{Provider: provider})

	result, err := a.Run(context.Background(), []chat.Message{chat.User("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FinalMessage.Text() != "ok" {
		t.Errorf("got %q, want %q", result.FinalMessage.Text(), "ok")
	}
}

// --- echoCountProvider: returns message count seen ---

type echoCountProvider struct{}

func (p *echoCountProvider) Name() string                       { return "echo-count" }
func (p *echoCountProvider) IsAvailable(_ context.Context) bool { return true }
func (p *echoCountProvider) Execute(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	text := fmt.Sprintf("seen %d messages", len(req.Messages))
	return llm.CompletionResponse{
		Message:    chat.Assistant(text),
		StopReason: chat.FinishReasonStop,
		Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
	}, nil
}

func (p *echoCountProvider) Stream(_ context.Context, req llm.CompletionRequest) (<-chan llm.StreamEvent, error) {
	resp, err := p.Execute(context.Background(), req)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamEvent, 1)
	go func() {
		defer close(ch)
		ch <- llm.MessageComplete{Response: resp}
	}()
	return ch, nil
}

func (p *echoCountProvider) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		Streaming:      true,
		MaxInputTokens: 100000,
	}
}

func (p *echoCountProvider) CountTokens(msgs []chat.Message) int {
	return chat.CountTokensApprox(msgs)
}
