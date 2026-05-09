package agent_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kbukum/gokit/ai/chat"

	"github.com/kbukum/gokit/agent"
	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/llm"
)

// --- InMemoryStore Tests ---

func TestInMemoryStore_LoadEmpty(t *testing.T) {
	store := agent.NewInMemoryStore()
	msgs, err := store.Load(context.Background(), "missing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgs != nil {
		t.Errorf("expected nil, got %v", msgs)
	}
}

func TestInMemoryStore_SaveAndLoad(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	original := []chat.Message{
		chat.User("hello"),
		chat.Assistant("hi there"),
	}

	if err := store.Save(ctx, "s1", original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := store.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(loaded))
	}

	// Verify content
	if u, ok := loaded[0].(chat.UserMessage); !ok {
		t.Errorf("msg[0] type = %T, want UserMessage", loaded[0])
	} else if ai.TextOf(u.Content) != "hello" {
		t.Errorf("msg[0] text = %q, want %q", ai.TextOf(u.Content), "hello")
	}

	if a, ok := loaded[1].(chat.AssistantMessage); !ok {
		t.Errorf("msg[1] type = %T, want AssistantMessage", loaded[1])
	} else if a.Text() != "hi there" {
		t.Errorf("msg[1] text = %q, want %q", a.Text(), "hi there")
	}
}

func TestInMemoryStore_DeepCopy(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	original := []chat.Message{chat.User("original")}
	if err := store.Save(ctx, "s1", original); err != nil {
		t.Fatal(err)
	}

	// Mutate the original slice after save
	original[0] = chat.User("mutated")

	loaded, _ := store.Load(ctx, "s1")
	if u, ok := loaded[0].(chat.UserMessage); !ok || ai.TextOf(u.Content) != "original" {
		t.Error("stored message was aliased with original")
	}

	// Mutate loaded slice
	loaded[0] = chat.User("also mutated")

	loaded2, _ := store.Load(ctx, "s1")
	if u, ok := loaded2[0].(chat.UserMessage); !ok || ai.TextOf(u.Content) != "original" {
		t.Error("stored message was aliased with loaded result")
	}
}

func TestInMemoryStore_Append(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	if err := store.Save(ctx, "s1", []chat.Message{chat.User("one")}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(ctx, "s1", chat.Assistant("two"), chat.User("three")); err != nil {
		t.Fatal(err)
	}

	loaded, _ := store.Load(ctx, "s1")
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}
}

func TestInMemoryStore_Clear(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	_ = store.Save(ctx, "s1", []chat.Message{chat.User("data")})
	if err := store.Clear(ctx, "s1"); err != nil {
		t.Fatal(err)
	}

	loaded, _ := store.Load(ctx, "s1")
	if loaded != nil {
		t.Errorf("expected nil after clear, got %v", loaded)
	}
}

func TestInMemoryStore_ConcurrentAccess(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			sid := fmt.Sprintf("session-%d", id%5)
			msg := chat.User(fmt.Sprintf("msg-%d", id))
			_ = store.Append(ctx, sid, msg)
			_, _ = store.Load(ctx, sid)
		}(i)
	}

	wg.Wait()

	// Verify no panics; check at least one session has data
	for i := 0; i < 5; i++ {
		loaded, err := store.Load(ctx, fmt.Sprintf("session-%d", i))
		if err != nil {
			t.Fatalf("error loading session-%d: %v", i, err)
		}
		if len(loaded) == 0 {
			t.Errorf("session-%d has no messages", i)
		}
	}
}

// --- SlidingWindowMemory Tests ---

func TestSlidingWindowMemory_WindowEnforcement(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	msgs := []chat.Message{
		chat.User("1"),
		chat.Assistant("2"),
		chat.User("3"),
		chat.Assistant("4"),
		chat.User("5"),
	}
	_ = store.Save(ctx, "s1", msgs)

	sw := agent.NewSlidingWindowMemory(store, 3)

	loaded, err := sw.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}

	// Should be the last 3: Assistant("2") is index 1, but last 3 are index 2,3,4
	if u, ok := loaded[0].(chat.UserMessage); !ok || ai.TextOf(u.Content) != "3" {
		t.Errorf("msg[0] = %T/%q, want UserMessage '3'", loaded[0], loaded[0])
	}
}

func TestSlidingWindowMemory_SystemMessagePreserved(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	msgs := []chat.Message{
		chat.System("system prompt"),
		chat.User("1"),
		chat.Assistant("2"),
		chat.User("3"),
		chat.Assistant("4"),
		chat.User("5"),
	}
	_ = store.Save(ctx, "s1", msgs)

	sw := agent.NewSlidingWindowMemory(store, 2)

	loaded, err := sw.Load(ctx, "s1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// system + last 2 = 3
	if len(loaded) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(loaded))
	}

	// First must be the system message
	if _, ok := loaded[0].(chat.SystemMessage); !ok {
		t.Errorf("first message should be SystemMessage, got %T", loaded[0])
	}
	// Last should be the 5th non-system message
	if u, ok := loaded[2].(chat.UserMessage); !ok || ai.TextOf(u.Content) != "5" {
		t.Errorf("last message should be User '5', got %T", loaded[2])
	}
}

func TestSlidingWindowMemory_SaveTrims(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	sw := agent.NewSlidingWindowMemory(store, 2)

	msgs := []chat.Message{
		chat.User("1"),
		chat.Assistant("2"),
		chat.User("3"),
		chat.Assistant("4"),
	}
	_ = sw.Save(ctx, "s1", msgs)

	// Load directly from underlying store to verify trimming happened
	raw, _ := store.Load(ctx, "s1")
	if len(raw) != 2 {
		t.Fatalf("expected 2 stored messages, got %d", len(raw))
	}
}

func TestSlidingWindowMemory_SmallHistory(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	_ = store.Save(ctx, "s1", []chat.Message{chat.User("only one")})

	sw := agent.NewSlidingWindowMemory(store, 10)
	loaded, _ := sw.Load(ctx, "s1")
	if len(loaded) != 1 {
		t.Fatalf("expected 1 message, got %d", len(loaded))
	}
}

func TestSlidingWindowMemory_Append(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	sw := agent.NewSlidingWindowMemory(store, 2)
	_ = sw.Append(ctx, "s1", chat.User("a"), chat.Assistant("b"), chat.User("c"))

	// Append delegates to underlying store, so all 3 are stored
	raw, _ := store.Load(ctx, "s1")
	if len(raw) != 3 {
		t.Fatalf("expected 3 stored messages, got %d", len(raw))
	}

	// Load through sliding window trims to 2
	loaded, _ := sw.Load(ctx, "s1")
	if len(loaded) != 2 {
		t.Fatalf("expected 2 messages from window load, got %d", len(loaded))
	}
}

func TestSlidingWindowMemory_Clear(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()

	sw := agent.NewSlidingWindowMemory(store, 5)
	_ = sw.Save(ctx, "s1", []chat.Message{chat.User("data")})
	_ = sw.Clear(ctx, "s1")

	loaded, _ := sw.Load(ctx, "s1")
	if loaded != nil {
		t.Errorf("expected nil after clear, got %v", loaded)
	}
}

// --- Integration: Agent Run with Memory ---

func TestAgent_RunWithMemory(t *testing.T) {
	store := agent.NewInMemoryStore()

	// Mock provider that echoes the number of input messages
	provider := &echoCountProvider{}

	a := agent.New(agent.Config{
		Provider:  provider,
		Memory:    store,
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

func TestAgent_RunWithSlidingWindowMemory(t *testing.T) {
	store := agent.NewInMemoryStore()
	sw := agent.NewSlidingWindowMemory(store, 4)

	provider := &echoCountProvider{}

	a := agent.New(agent.Config{
		Provider:  provider,
		Memory:    sw,
		SessionID: "windowed",
	})
	ctx := context.Background()

	// Run three conversations to accumulate history
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
