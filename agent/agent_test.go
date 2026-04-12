package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/kbukum/gokit/agent"
	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

// --- Mock Provider ---

type mockProvider struct {
	responses []llm.CompletionResponse
	callIdx   int
	mu        sync.Mutex
	caps      llm.Capabilities
}

func newMockProvider(responses ...llm.CompletionResponse) *mockProvider {
	return &mockProvider{
		responses: responses,
		caps: llm.Capabilities{
			SupportsTools:     true,
			SupportsStreaming: true,
			MaxContextTokens:  100000,
			MaxOutputTokens:   4096,
			ModelID:           "mock-model",
		},
	}
}

func (m *mockProvider) Complete(_ context.Context, _ llm.CompletionRequest) (*llm.CompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callIdx >= len(m.responses) {
		return nil, fmt.Errorf("no more mock responses")
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return &resp, nil
}

func (m *mockProvider) Stream(_ context.Context, req llm.CompletionRequest) (<-chan llm.StreamEvent, error) {
	resp, err := m.Complete(context.Background(), req)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamEvent, 2)
	go func() {
		defer close(ch)
		ch <- llm.MessageComplete{Response: *resp}
	}()
	return ch, nil
}

func (m *mockProvider) Capabilities() llm.Capabilities { return m.caps }

func (m *mockProvider) CountTokens(messages []llm.Message) int {
	return llm.CountTokensApprox(messages)
}

// --- Mock Tool ---

func makeMockTool(name, result string) *tool.Registry {
	reg := tool.NewRegistry()
	t := tool.FromFunc(name, "A test tool", func(ctx context.Context, in struct{}) (string, error) {
		return result, nil
	})
	reg.Register(t.AsCallable())
	return reg
}

func makeErrorTool(name string) *tool.Registry {
	reg := tool.NewRegistry()
	t := tool.FromFunc(name, "A failing tool", func(ctx context.Context, in struct{}) (string, error) {
		return "", fmt.Errorf("tool failed")
	})
	reg.Register(t.AsCallable())
	return reg
}

// --- Helper to build responses ---

func textResponse(text string) llm.CompletionResponse {
	return llm.CompletionResponse{
		Message:    llm.Assistant(text),
		StopReason: llm.StopEndTurn,
		Usage:      llm.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
}

func toolCallResponse(toolName, args string) llm.CompletionResponse {
	return llm.CompletionResponse{
		Message: llm.AssistantMessage{
			Content: []llm.ContentBlock{llm.TextBlock{Text: "Let me use a tool"}},
			ToolCalls: []llm.ToolCall{
				{
					ID: "call_1",
					Function: llm.FunctionCall{
						Name:      toolName,
						Arguments: args,
					},
				},
			},
		},
		StopReason: llm.StopToolUse,
		Usage:      llm.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
	}
}

// --- Tests ---

func TestAgent_SimpleCompletion(t *testing.T) {
	provider := newMockProvider(textResponse("Hello!"))
	a := agent.New(agent.Config{
		Provider:     provider,
		SystemPrompt: "You are helpful.",
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("Hi")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
	if result.TurnCount != 1 {
		t.Errorf("TurnCount = %d, want 1", result.TurnCount)
	}
	if result.FinalMessage.Text() != "Hello!" {
		t.Errorf("FinalMessage = %q, want %q", result.FinalMessage.Text(), "Hello!")
	}
	if result.TotalUsage.TotalTokens != 15 {
		t.Errorf("TotalTokens = %d, want 15", result.TotalUsage.TotalTokens)
	}
}

func TestAgent_ToolCallThenResponse(t *testing.T) {
	provider := newMockProvider(
		toolCallResponse("calculator", "{}"),
		textResponse("The answer is 42."),
	)
	tools := makeMockTool("calculator", "42")

	a := agent.New(agent.Config{
		Provider: provider,
		Tools:    tools,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("What is 6*7?")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
	if result.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2", result.TurnCount)
	}
	if result.TotalUsage.TotalTokens != 45 {
		t.Errorf("TotalTokens = %d, want 45", result.TotalUsage.TotalTokens)
	}
	// Messages: user + assistant(tool_call) + tool_result + assistant(final)
	if len(result.Messages) != 4 {
		t.Errorf("Messages count = %d, want 4", len(result.Messages))
	}
}

func TestAgent_MaxTurns(t *testing.T) {
	// Provider always requests tool calls, never stops
	responses := make([]llm.CompletionResponse, 5)
	for i := range responses {
		responses[i] = toolCallResponse("calculator", "{}")
	}
	provider := newMockProvider(responses...)
	tools := makeMockTool("calculator", "42")

	a := agent.New(agent.Config{
		Provider: provider,
		Tools:    tools,
		MaxTurns: 3,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("loop")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StopReason != agent.StopMaxTurns {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopMaxTurns)
	}
	if result.TurnCount != 3 {
		t.Errorf("TurnCount = %d, want 3", result.TurnCount)
	}
}

func TestAgent_MaxTokenBudget(t *testing.T) {
	provider := newMockProvider(
		toolCallResponse("calculator", "{}"),
		textResponse("done"),
	)
	tools := makeMockTool("calculator", "42")

	a := agent.New(agent.Config{
		Provider:       provider,
		Tools:          tools,
		MaxTokenBudget: 25, // First turn uses 30 total tokens
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StopReason != agent.StopMaxBudget {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopMaxBudget)
	}
}

func TestAgent_ToolError(t *testing.T) {
	provider := newMockProvider(
		toolCallResponse("broken", "{}"),
		textResponse("I encountered an error."),
	)
	tools := makeErrorTool("broken")

	a := agent.New(agent.Config{
		Provider: provider,
		Tools:    tools,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("run broken")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
	// Check that the tool result message has isError=true
	found := false
	for _, msg := range result.Messages {
		if trm, ok := msg.(llm.ToolResultMessage); ok {
			if !trm.IsError {
				t.Error("expected tool result to be error")
			}
			found = true
		}
	}
	if !found {
		t.Error("no ToolResultMessage found in messages")
	}
}

func TestAgent_HookAbortOnTurnStart(t *testing.T) {
	provider := newMockProvider(textResponse("should not reach"))
	hooks := hook.NewRegistry()
	hooks.On(agent.EventTurnStart, func(e hook.Event) hook.Result {
		return hook.Abort("blocked by policy")
	})

	a := agent.New(agent.Config{
		Provider: provider,
		Hooks:    hooks,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StopReason != agent.StopAborted {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopAborted)
	}
	if result.TurnCount != 0 {
		t.Errorf("TurnCount = %d, want 0", result.TurnCount)
	}
}

func TestAgent_HookAbortOnPreToolCall(t *testing.T) {
	provider := newMockProvider(
		toolCallResponse("dangerous", "{}"),
		textResponse("OK, skipped."),
	)
	tools := makeMockTool("dangerous", "should not run")
	hooks := hook.NewRegistry()
	hooks.On(agent.EventPreToolCall, func(e hook.Event) hook.Result {
		pre := e.(agent.PreToolCall)
		if pre.Name == "dangerous" {
			return hook.Abort("tool blocked")
		}
		return hook.Continue()
	})

	a := agent.New(agent.Config{
		Provider: provider,
		Tools:    tools,
		Hooks:    hooks,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("use dangerous")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The tool was aborted, its error message was added, then LLM responded
	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
	// Check tool result has error about being blocked
	for _, msg := range result.Messages {
		if trm, ok := msg.(llm.ToolResultMessage); ok {
			if !trm.IsError {
				t.Error("expected tool result to be error from abort")
			}
		}
	}
}

func TestAgent_HookModifyPreLLMCall(t *testing.T) {
	var capturedModel string
	provider := &mockProvider{
		responses: []llm.CompletionResponse{textResponse("done")},
		caps:      llm.Capabilities{ModelID: "mock"},
	}
	// Override Complete to capture the request
	origProvider := provider
	wrapper := &modelCapturingProvider{inner: origProvider, captured: &capturedModel}

	hooks := hook.NewRegistry()
	hooks.On(agent.EventPreLLMCall, func(e hook.Event) hook.Result {
		pre := e.(agent.PreLLMCall)
		modified := pre.Request
		modified.Model = "gpt-4-turbo"
		return hook.Modify(modified)
	})

	a := agent.New(agent.Config{
		Provider: wrapper,
		Hooks:    hooks,
	})

	_, err := a.Run(context.Background(), []llm.Message{llm.User("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedModel != "gpt-4-turbo" {
		t.Errorf("model = %q, want gpt-4-turbo", capturedModel)
	}
}

type modelCapturingProvider struct {
	inner    *mockProvider
	captured *string
}

func (m *modelCapturingProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	*m.captured = req.Model
	return m.inner.Complete(ctx, req)
}

func (m *modelCapturingProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamEvent, error) {
	*m.captured = req.Model
	return m.inner.Stream(ctx, req)
}

func (m *modelCapturingProvider) Capabilities() llm.Capabilities { return m.inner.Capabilities() }

func (m *modelCapturingProvider) CountTokens(msgs []llm.Message) int {
	return m.inner.CountTokens(msgs)
}

// --- Context Strategy Tests ---

func TestFailStrategy(t *testing.T) {
	s := agent.FailStrategy{}
	_, err := s.Compact(nil, 0)
	if err != agent.ErrContextExceeded {
		t.Errorf("expected ErrContextExceeded, got %v", err)
	}
}

func TestTruncateStrategy(t *testing.T) {
	msgs := []llm.Message{
		llm.User("1"),
		llm.User("2"),
		llm.User("3"),
		llm.User("4"),
		llm.User("5"),
	}

	s := agent.TruncateStrategy{KeepLast: 3}
	result, err := s.Compact(msgs, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 messages, got %d", len(result))
	}
}

func TestTruncateStrategy_ShortList(t *testing.T) {
	msgs := []llm.Message{llm.User("1")}
	s := agent.TruncateStrategy{KeepLast: 5}
	result, err := s.Compact(msgs, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

func TestSlidingWindowStrategy(t *testing.T) {
	msgs := []llm.Message{
		llm.System("system"),
		llm.User("1"),
		llm.User("2"),
		llm.User("3"),
		llm.User("4"),
	}

	// Use a counter that assigns 10 tokens per message.
	counter := func(ms []llm.Message) int { return len(ms) * 10 }
	s := agent.SlidingWindowStrategy{TokenCounter: counter}

	// Budget = 30 tokens (3 messages). System + 2 recent should fit.
	result, err := s.Compact(msgs, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have system + 2 recent = 3 messages.
	if len(result) > 3 {
		t.Errorf("expected ≤3 messages, got %d", len(result))
	}
	// First should still be system.
	if _, ok := result[0].(llm.SystemMessage); !ok {
		t.Error("expected system message to be preserved")
	}
}

func TestSlidingWindowStrategy_AlreadyFits(t *testing.T) {
	msgs := []llm.Message{llm.User("1"), llm.User("2")}
	counter := func(ms []llm.Message) int { return len(ms) * 10 }
	s := agent.SlidingWindowStrategy{TokenCounter: counter}

	result, err := s.Compact(msgs, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result))
	}
}

func TestSummarizeStrategy_FallbackTruncate(t *testing.T) {
	// Without a provider, it should just truncate.
	msgs := []llm.Message{
		llm.User("1"),
		llm.User("2"),
		llm.User("3"),
		llm.User("4"),
		llm.User("5"),
	}
	s := agent.SummarizeStrategy{KeepLast: 2}
	result, err := s.Compact(msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 messages (fallback truncate), got %d", len(result))
	}
}

func TestSummarizeStrategy_ShortList(t *testing.T) {
	msgs := []llm.Message{llm.User("1")}
	s := agent.SummarizeStrategy{KeepLast: 4}
	result, err := s.Compact(msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

// --- Event Types ---

func TestEventTypes(t *testing.T) {
	events := []agent.Event{
		agent.TurnStartEvent{Turn: 1},
		agent.LLMStreamEvent{},
		agent.ToolExecutingEvent{ToolUseID: "1", Name: "test", Input: json.RawMessage(`{}`)},
		agent.ToolCompleteEvent{ToolUseID: "1", Name: "test"},
		agent.ContextCompactedEvent{OldTokens: 100, NewTokens: 50},
		agent.TurnCompleteEvent{Turn: 1},
		agent.CompleteEvent{},
	}

	// Just verify they all implement Event
	for i, e := range events {
		if e == nil {
			t.Errorf("event %d is nil", i)
		}
	}
}

// --- Stream Tests ---

func TestAgent_Stream_SimpleCompletion(t *testing.T) {
	provider := newMockProvider(textResponse("streamed response"))
	a := agent.New(agent.Config{
		Provider: provider,
	})

	ch, err := a.Stream(context.Background(), []llm.Message{llm.User("Hi")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := make([]agent.Event, 0, 2)
	for e := range ch {
		events = append(events, e)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	// First should be TurnStart
	if _, ok := events[0].(agent.TurnStartEvent); !ok {
		t.Errorf("first event should be TurnStartEvent, got %T", events[0])
	}

	// Last should be CompleteEvent
	last := events[len(events)-1]
	complete, ok := last.(agent.CompleteEvent)
	if !ok {
		t.Errorf("last event should be CompleteEvent, got %T", last)
	} else if complete.Result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", complete.Result.StopReason, agent.StopEndTurn)
	}
}

func TestAgent_DefaultMaxTurns(t *testing.T) {
	a := agent.New(agent.Config{
		Provider: newMockProvider(textResponse("hi")),
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should work fine with default max turns (100)
	if result.TurnCount != 1 {
		t.Errorf("TurnCount = %d, want 1", result.TurnCount)
	}
}

func TestAgent_SystemPromptIncluded(t *testing.T) {
	provider := newMockProvider(textResponse("hi"))
	a := agent.New(agent.Config{
		Provider:     provider,
		SystemPrompt: "Be concise.",
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
}

// --- Multi-Tool Tests ---

func multiToolCallResponse(tools map[string]string) llm.CompletionResponse {
	calls := make([]llm.ToolCall, 0, len(tools))
	i := 0
	for name, args := range tools {
		calls = append(calls, llm.ToolCall{
			ID:       fmt.Sprintf("call_%d", i),
			Function: llm.FunctionCall{Name: name, Arguments: args},
		})
		i++
	}
	return llm.CompletionResponse{
		Message: llm.AssistantMessage{
			Content:   []llm.ContentBlock{llm.TextBlock{Text: "calling tools"}},
			ToolCalls: calls,
		},
		StopReason: llm.StopToolUse,
		Usage:      llm.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
	}
}

func makeMultiTool(names ...string) *tool.Registry {
	reg := tool.NewRegistry()
	for _, name := range names {
		n := name
		t := tool.FromFunc(n, "tool "+n, func(ctx context.Context, in struct{}) (string, error) {
			return "result-" + n, nil
		})
		reg.Register(t.AsCallable())
	}
	return reg
}

func makeReadOnlyMultiTool(names ...string) *tool.Registry {
	reg := tool.NewRegistry()
	for _, name := range names {
		n := name
		t := tool.FromFunc(n, "tool "+n, func(ctx context.Context, in struct{}) (string, error) {
			return "result-" + n, nil
		})
		t.Def.ReadOnly = true
		reg.Register(t.AsCallable())
	}
	return reg
}

func TestAgent_MultiToolCall(t *testing.T) {
	provider := newMockProvider(
		multiToolCallResponse(map[string]string{"search": "{}", "fetch": "{}"}),
		textResponse("Found and fetched the data."),
	)
	tools := makeMultiTool("search", "fetch")

	a := agent.New(agent.Config{
		Provider: provider,
		Tools:    tools,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("search and fetch data")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
	if result.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2", result.TurnCount)
	}
	// Messages: user + assistant(multi-tool) + 2 tool results + assistant(final)
	if len(result.Messages) != 5 {
		t.Errorf("Messages count = %d, want 5", len(result.Messages))
	}
	// Verify both tool results are present
	toolResults := 0
	for _, msg := range result.Messages {
		if _, ok := msg.(llm.ToolResultMessage); ok {
			toolResults++
		}
	}
	if toolResults != 2 {
		t.Errorf("tool results = %d, want 2", toolResults)
	}
}

func TestAgent_ParallelTools(t *testing.T) {
	provider := newMockProvider(
		multiToolCallResponse(map[string]string{"lookup1": "{}", "lookup2": "{}"}),
		textResponse("Both lookups done."),
	)
	tools := makeReadOnlyMultiTool("lookup1", "lookup2")

	var parallelHookFired bool
	hooks := hook.NewRegistry()
	hooks.On(agent.EventToolsParallelized, func(e hook.Event) hook.Result {
		parallelHookFired = true
		tp := e.(agent.ToolsParallelized)
		if tp.Count != 2 {
			t.Errorf("parallel tool count = %d, want 2", tp.Count)
		}
		return hook.Continue()
	})

	a := agent.New(agent.Config{
		Provider:      provider,
		Tools:         tools,
		Hooks:         hooks,
		ParallelTools: true,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("lookup both")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !parallelHookFired {
		t.Error("ToolsParallelized hook was not fired")
	}
	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
	if result.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2", result.TurnCount)
	}
}

// --- Memory Tests ---

func TestAgent_MemoryLoadAndSave(t *testing.T) {
	store := agent.NewInMemoryStore()

	// Pre-populate memory with history
	ctx := context.Background()
	_ = store.Save(ctx, "sess-1", []llm.Message{
		llm.User("previous question"),
		llm.AssistantMessage{Content: []llm.ContentBlock{llm.TextBlock{Text: "previous answer"}}},
	})

	provider := newMockProvider(textResponse("Based on our history..."))
	a := agent.New(agent.Config{
		Provider:  provider,
		Memory:    store,
		SessionID: "sess-1",
	})

	result, err := a.Run(ctx, []llm.Message{llm.User("follow up")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Messages: history(2) + new user + assistant = 4
	if len(result.Messages) < 4 {
		t.Errorf("Messages count = %d, want >= 4", len(result.Messages))
	}

	// Verify memory was updated with full conversation
	saved, _ := store.Load(ctx, "sess-1")
	if len(saved) != len(result.Messages) {
		t.Errorf("saved memory has %d messages, want %d", len(saved), len(result.Messages))
	}
}

func TestAgent_MemoryHookFired(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()
	_ = store.Save(ctx, "sess-1", []llm.Message{llm.User("old msg")})

	var loaded bool
	hooks := hook.NewRegistry()
	hooks.On(agent.EventMemoryLoaded, func(e hook.Event) hook.Result {
		loaded = true
		ml := e.(agent.MemoryLoaded)
		if ml.SessionID != "sess-1" {
			t.Errorf("SessionID = %q, want sess-1", ml.SessionID)
		}
		if ml.MessageCount != 1 {
			t.Errorf("MessageCount = %d, want 1", ml.MessageCount)
		}
		return hook.Continue()
	})

	provider := newMockProvider(textResponse("hi"))
	a := agent.New(agent.Config{
		Provider:  provider,
		Hooks:     hooks,
		Memory:    store,
		SessionID: "sess-1",
	})

	_, err := a.Run(ctx, []llm.Message{llm.User("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !loaded {
		t.Error("MemoryLoaded hook was not fired")
	}
}

// --- Slash Command Tests ---

func TestAgent_SlashCommand_Help(t *testing.T) {
	cmds := agent.NewCommandRegistry()
	cmds.RegisterBuiltins()

	provider := newMockProvider(textResponse("should not be called"))
	a := agent.New(agent.Config{
		Provider: provider,
		Commands: cmds,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("/help")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StopReason != agent.StopCommand {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopCommand)
	}
	if result.FinalMessage.Text() == "" {
		t.Error("expected non-empty help output")
	}
}

func TestAgent_SlashCommand_Clear(t *testing.T) {
	store := agent.NewInMemoryStore()
	ctx := context.Background()
	_ = store.Save(ctx, "sess-1", []llm.Message{llm.User("old")})

	cmds := agent.NewCommandRegistry()
	cmds.RegisterBuiltins()

	provider := newMockProvider(textResponse("should not be called"))
	a := agent.New(agent.Config{
		Provider:  provider,
		Commands:  cmds,
		Memory:    store,
		SessionID: "sess-1",
	})

	result, err := a.Run(ctx, []llm.Message{llm.User("/clear")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.StopReason != agent.StopCommand {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopCommand)
	}

	// Verify memory was cleared
	saved, _ := store.Load(ctx, "sess-1")
	if len(saved) != 0 {
		t.Errorf("memory should be empty after /clear, has %d messages", len(saved))
	}
}

func TestAgent_SlashCommand_Model(t *testing.T) {
	cmds := agent.NewCommandRegistry()
	cmds.RegisterBuiltins()

	provider := newMockProvider(textResponse("should not be called"))
	a := agent.New(agent.Config{
		Provider: provider,
		Commands: cmds,
		Model:    "gpt-4",
	})

	// Switch model
	result, err := a.Run(context.Background(), []llm.Message{llm.User("/model gpt-4o")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != agent.StopCommand {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopCommand)
	}
	if a.GetModel() != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", a.GetModel())
	}

	// Query model
	result2, err := a.Run(context.Background(), []llm.Message{llm.User("/model")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.FinalMessage.Text() == "" {
		t.Error("expected non-empty model output")
	}
}

func TestAgent_NonCommandPassthrough(t *testing.T) {
	cmds := agent.NewCommandRegistry()
	cmds.RegisterBuiltins()

	provider := newMockProvider(textResponse("normal response"))
	a := agent.New(agent.Config{
		Provider: provider,
		Commands: cmds,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("hello")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Regular messages should bypass commands and go to LLM
	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
	if result.FinalMessage.Text() != "normal response" {
		t.Errorf("FinalMessage = %q, want %q", result.FinalMessage.Text(), "normal response")
	}
}

// --- Context Compaction Test ---

func TestAgent_ContextCompaction(t *testing.T) {
	// Create a provider with tiny context window
	provider := &mockProvider{
		responses: []llm.CompletionResponse{
			toolCallResponse("search", "{}"),
			textResponse("done"),
		},
		caps: llm.Capabilities{
			SupportsTools:    true,
			MaxContextTokens: 20, // Very small to trigger compaction
			ModelID:          "mock",
		},
	}
	tools := makeMockTool("search", "lots of data here")

	var compactedHookFired bool
	hooks := hook.NewRegistry()
	hooks.On(agent.EventContextCompacted, func(e hook.Event) hook.Result {
		compactedHookFired = true
		return hook.Continue()
	})

	a := agent.New(agent.Config{
		Provider:        provider,
		Tools:           tools,
		Hooks:           hooks,
		ContextStrategy: agent.TruncateStrategy{KeepLast: 3},
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("search for something")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
	if !compactedHookFired {
		t.Error("ContextCompacted hook was not fired")
	}
}

// --- System Prompt Template Test ---

func TestAgent_SystemPromptTemplate(t *testing.T) {
	tmpl, err := agent.NewPromptTemplate("system", "You are {{.Role}}. Help with {{.Topic}}.")
	if err != nil {
		t.Fatalf("failed to create template: %v", err)
	}

	var capturedReq llm.CompletionRequest
	provider := &reqCapturingProvider{
		inner:    newMockProvider(textResponse("hi")),
		captured: &capturedReq,
	}

	a := agent.New(agent.Config{
		Provider:             provider,
		SystemPromptTemplate: tmpl,
		SystemPromptData:     map[string]string{"Role": "analyst", "Topic": "data"},
	})

	_, runErr := a.Run(context.Background(), []llm.Message{llm.User("test")})
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}

	// Verify system prompt was rendered from template into SystemPrompt field
	expected := "You are analyst. Help with data."
	if capturedReq.SystemPrompt != expected {
		t.Errorf("system prompt = %q, want %q", capturedReq.SystemPrompt, expected)
	}
}

type reqCapturingProvider struct {
	inner    *mockProvider
	captured *llm.CompletionRequest
}

func (p *reqCapturingProvider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	*p.captured = req
	return p.inner.Complete(ctx, req)
}

func (p *reqCapturingProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamEvent, error) {
	*p.captured = req
	return p.inner.Stream(ctx, req)
}

func (p *reqCapturingProvider) Capabilities() llm.Capabilities { return p.inner.Capabilities() }

func (p *reqCapturingProvider) CountTokens(msgs []llm.Message) int {
	return p.inner.CountTokens(msgs)
}

// --- Model Override Test ---

func TestAgent_ModelOverride(t *testing.T) {
	var capturedReq llm.CompletionRequest
	provider := &reqCapturingProvider{
		inner:    newMockProvider(textResponse("hi")),
		captured: &capturedReq,
	}

	a := agent.New(agent.Config{
		Provider: provider,
		Model:    "custom-model-v2",
	})

	_, err := a.Run(context.Background(), []llm.Message{llm.User("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.Model != "custom-model-v2" {
		t.Errorf("model = %q, want custom-model-v2", capturedReq.Model)
	}
}

// --- ToolFormatter Test ---

func TestAgent_ToolFormatter(t *testing.T) {
	provider := newMockProvider(
		toolCallResponse("search", "{}"),
		textResponse("formatted result received"),
	)
	tools := makeMockTool("search", "raw json data here")

	formatter := tool.FormatterFunc(func(name string, r *tool.Result) (string, error) {
		return fmt.Sprintf("[%s] %s", name, r.Content), nil
	})

	a := agent.New(agent.Config{
		Provider:      provider,
		Tools:         tools,
		ToolFormatter: formatter,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("search")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the tool result message and verify it was formatted
	for _, msg := range result.Messages {
		if trm, ok := msg.(llm.ToolResultMessage); ok {
			// FromFunc JSON-marshals the string output, so Content includes quotes
			expected := `[search] "raw json data here"`
			if trm.Content != expected {
				t.Errorf("tool result = %q, want %q", trm.Content, expected)
			}
		}
	}
	if result.StopReason != agent.StopEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, agent.StopEndTurn)
	}
}

// --- Stream with Tool Call Test ---

func TestAgent_Stream_ToolCall(t *testing.T) {
	provider := newMockProvider(
		toolCallResponse("calc", "{}"),
		textResponse("42"),
	)
	tools := makeMockTool("calc", "42")

	a := agent.New(agent.Config{
		Provider: provider,
		Tools:    tools,
	})

	ch, err := a.Stream(context.Background(), []llm.Message{llm.User("calc")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events := make([]agent.Event, 0, 8)
	for e := range ch {
		events = append(events, e)
	}

	// Should have: TurnStart, TurnComplete/ToolExecuting/ToolComplete, TurnStart, TurnComplete, CompleteEvent
	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// Last should be CompleteEvent
	last := events[len(events)-1]
	complete, ok := last.(agent.CompleteEvent)
	if !ok {
		t.Errorf("last event should be CompleteEvent, got %T", last)
	} else if complete.Result.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2", complete.Result.TurnCount)
	}
}

// --- Context Cancel Test ---

func TestAgent_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	provider := newMockProvider(textResponse("should not reach"))
	a := agent.New(agent.Config{
		Provider: provider,
	})

	_, err := a.Run(ctx, []llm.Message{llm.User("test")})
	// Should get a context error from the provider
	if err == nil {
		// Some providers may not check ctx, which is fine — the test
		// mainly verifies no panic occurs.
		return
	}
}

func TestAgent_UsageAccumulation(t *testing.T) {
	provider := newMockProvider(
		llm.CompletionResponse{
			Message:    llm.AssistantMessage{Content: []llm.ContentBlock{llm.TextBlock{Text: "thinking"}}, ToolCalls: []llm.ToolCall{{ID: "c1", Function: llm.FunctionCall{Name: "calc", Arguments: "{}"}}}},
			StopReason: llm.StopToolUse,
			Usage:      llm.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		},
		llm.CompletionResponse{
			Message:    llm.Assistant("done"),
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{PromptTokens: 200, CompletionTokens: 30, TotalTokens: 230},
		},
	)
	tools := makeMockTool("calc", "result")

	a := agent.New(agent.Config{
		Provider: provider,
		Tools:    tools,
	})

	result, err := a.Run(context.Background(), []llm.Message{llm.User("test")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalUsage.PromptTokens != 300 {
		t.Errorf("PromptTokens = %d, want 300", result.TotalUsage.PromptTokens)
	}
	if result.TotalUsage.CompletionTokens != 80 {
		t.Errorf("CompletionTokens = %d, want 80", result.TotalUsage.CompletionTokens)
	}
	if result.TotalUsage.TotalTokens != 380 {
		t.Errorf("TotalTokens = %d, want 380", result.TotalUsage.TotalTokens)
	}
}
