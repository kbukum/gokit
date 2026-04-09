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
			SupportsTools:    true,
			SupportsStreaming: true,
			MaxContextTokens: 100000,
			MaxOutputTokens:  4096,
			ModelID:          "mock-model",
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
	hooks.On(hook.EventTurnStart, func(e hook.Event) hook.Result {
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
	hooks.On(hook.EventPreToolCall, func(e hook.Event) hook.Result {
		pre := e.(hook.PreToolCall)
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
	hooks.On(hook.EventPreLLMCall, func(e hook.Event) hook.Result {
		pre := e.(hook.PreLLMCall)
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

	var events []agent.Event
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
