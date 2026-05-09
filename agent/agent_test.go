package agent_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kbukum/gokit/ai/chat"

	"github.com/kbukum/gokit/agent"
	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/prompt"
	"github.com/kbukum/gokit/hook"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/tool"
)

type mockProvider struct {
	responses        []llm.CompletionResponse
	callIdx          int
	mu               sync.Mutex
	caps             llm.Capabilities
	blockUntilCancel bool
}

func newMockProvider(responses ...llm.CompletionResponse) *mockProvider {
	return &mockProvider{responses: responses, caps: llm.Capabilities{ToolUse: true, Streaming: true, MaxInputTokens: 100000, MaxOutputTokens: 4096}}
}
func (m *mockProvider) Name() string                       { return "mock" }
func (m *mockProvider) IsAvailable(_ context.Context) bool { return true }
func (m *mockProvider) Execute(ctx context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	if m.blockUntilCancel {
		<-ctx.Done()
		return llm.CompletionResponse{}, ctx.Err()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callIdx >= len(m.responses) {
		return llm.CompletionResponse{}, fmt.Errorf("no more mock responses")
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

func (m *mockProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamEvent, error) {
	resp, err := m.Execute(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamEvent, 2)
	go func() {
		defer close(ch)
		ch <- llm.UsageDelta{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
		ch <- llm.MessageComplete{Response: resp}
	}()
	return ch, nil
}
func (m *mockProvider) Capabilities() llm.Capabilities { return m.caps }
func (m *mockProvider) CountTokens(messages []chat.Message) int {
	return chat.CountTokensApprox(messages)
}

func textResponse(text string) llm.CompletionResponse {
	return llm.CompletionResponse{Message: chat.Assistant(text), StopReason: chat.FinishReasonStop, Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}}
}

func toolCallResponse(toolName, _ string) llm.CompletionResponse {
	return llm.CompletionResponse{Message: chat.AssistantMessage{Content: []ai.ContentPart{ai.Text{Text: "use tool"}}, ToolCalls: []ai.ToolUseBlock{{ID: "call_1", Name: toolName, Input: map[string]any{}}}}, StopReason: chat.FinishReasonToolUse, Usage: llm.Usage{InputTokens: 20, OutputTokens: 10}}
}

func makeMockTool(name, result string) *tool.Registry {
	reg := tool.NewRegistry()
	t := tool.FromFunc(name, "test tool", func(ctx context.Context, in struct{}) (string, error) { return result, nil })
	_ = reg.Register(t.AsCallable())
	return reg
}

func TestAgentSimpleCompletion(t *testing.T) {
	p := newMockProvider(textResponse("Hello!"))
	a := agent.New(agent.Config{Provider: p, SystemPrompt: "helpful"})
	r, err := a.Run(context.Background(), []chat.Message{chat.User("Hi")})
	if err != nil {
		t.Fatal(err)
	}
	if r.StopReason != agent.StopEndTurn || r.FinalMessage.Text() != "Hello!" || r.TurnCount != 1 {
		t.Fatalf("unexpected result: %#v", r)
	}
}

func TestAgentToolCallThenResponse(t *testing.T) {
	p := newMockProvider(toolCallResponse("calculator", "{}"), textResponse("42"))
	a := agent.New(agent.Config{Provider: p, Tools: makeMockTool("calculator", "42")})
	r, err := a.Run(context.Background(), []chat.Message{chat.User("calc")})
	if err != nil {
		t.Fatal(err)
	}
	if r.StopReason != agent.StopEndTurn || r.TurnCount != 2 {
		t.Fatalf("unexpected result: %#v", r)
	}
}

func TestAgentMaxTurnsTypedError(t *testing.T) {
	p := newMockProvider(toolCallResponse("calculator", "{}"), toolCallResponse("calculator", "{}"))
	a := agent.New(agent.Config{Provider: p, Tools: makeMockTool("calculator", "42"), MaxTurns: 1})
	r, err := a.Run(context.Background(), []chat.Message{chat.User("loop")})
	if !errors.Is(err, agent.ErrMaxTurnsExceeded) {
		t.Fatalf("got err %v", err)
	}
	if r.StopReason != agent.StopMaxTurns {
		t.Fatalf("stop=%s", r.StopReason)
	}
}

func TestAgentMaxTokensTypedError(t *testing.T) {
	p := newMockProvider(toolCallResponse("calculator", "{}"))
	a := agent.New(agent.Config{Provider: p, Tools: makeMockTool("calculator", "42"), MaxTokens: 25})
	r, err := a.Run(context.Background(), []chat.Message{chat.User("test")})
	if !errors.Is(err, agent.ErrMaxTokensExceeded) {
		t.Fatalf("got err %v", err)
	}
	if r.StopReason != agent.StopMaxTokens {
		t.Fatalf("stop=%s", r.StopReason)
	}
}

func TestAgentMaxToolCallsTypedError(t *testing.T) {
	p := newMockProvider(toolCallResponse("calculator", "{}"))
	a := agent.New(agent.Config{Provider: p, Tools: makeMockTool("calculator", "42"), MaxToolCalls: 1, MaxTurns: 1})
	_, err := a.Run(context.Background(), []chat.Message{chat.User("test")})
	if !errors.Is(err, agent.ErrMaxTurnsExceeded) {
		t.Fatalf("turn boundary after one tool call got %v", err)
	}
}

func TestAgentCancellationWithinOneSecond(t *testing.T) {
	p := newMockProvider()
	p.blockUntilCancel = true
	a := agent.New(agent.Config{Provider: p, WallClock: 10 * time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := a.Run(ctx, []chat.Message{chat.User("wait")}); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, agent.ErrCancelled) && !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not propagate within 1s")
	}
}

func TestAgentSystemPromptTemplate(t *testing.T) {
	tmpl, err := prompt.NewTemplate("sys", "You help with {{Topic}}.")
	if err != nil {
		t.Fatal(err)
	}
	p := newMockProvider(textResponse("ok"))
	a := agent.New(agent.Config{Provider: p, SystemPromptTemplate: tmpl, SystemPromptData: map[string]string{"Topic": "math"}})
	if _, err := a.Run(context.Background(), []chat.Message{chat.User("hi")}); err != nil {
		t.Fatal(err)
	}
}

func TestAgentStreamExposesLLMEvents(t *testing.T) {
	p := newMockProvider(textResponse("ok"))
	a := agent.New(agent.Config{Provider: p})
	ch, err := a.Stream(context.Background(), []chat.Message{chat.User("hi")})
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for evt := range ch {
		if _, ok := evt.(llm.MessageComplete); ok {
			seen = true
		}
	}
	if !seen {
		t.Fatal("missing message.complete")
	}
}

func TestMemoryPoliciesAndHookTypes(t *testing.T) {
	msgs := []chat.Message{chat.System("sys"), chat.User("1"), chat.User("2"), chat.User("3")}
	got, err := (agent.RingBufferPolicy{KeepLast: 2}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("ring got %d", len(got))
	}
	got, err = (agent.TruncateStrategy{KeepLast: 2}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("truncate got %d", len(got))
	}
	got, err = (agent.SlidingWindowStrategy{TokenCounter: func([]chat.Message) int { return 1 }}).Compact(context.Background(), msgs, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("empty sliding window")
	}
	events := []interface{ Type() hook.EventType }{agent.StartEvent{}, agent.LLMRequestEvent{}, agent.LLMResponseEvent{}, agent.ToolCallEvent{}, agent.ToolResultEvent{}, agent.MCPRequestEvent{}, agent.MCPResultEvent{}, agent.StreamObservedEvent{}, agent.StepCompleteEvent{}, agent.ErrorEvent{}, agent.StopEvent{}, agent.ContextCompacted{}, agent.ModelSwitched{}, agent.MemoryLoaded{}}
	for _, e := range events {
		if e.Type() == "" {
			t.Fatalf("empty type for %T", e)
		}
	}
}

func TestAgentErrorAndHookPaths(t *testing.T) {
	p := newMockProvider()
	a := agent.New(agent.Config{Provider: p})
	if _, err := a.Run(context.Background(), []chat.Message{chat.User("hi")}); err == nil {
		t.Fatal("expected provider error")
	}
	reg := hook.NewRegistry()
	reg.On(agent.EventOnStart, func(context.Context, hook.Event) error { return errors.Join(errors.New("stop"), hook.ErrFatalHook) })
	a = agent.New(agent.Config{Provider: newMockProvider(textResponse("unused")), Hooks: reg})
	if _, err := a.Run(context.Background(), []chat.Message{chat.User("hi")}); err == nil || !errors.Is(err, hook.ErrFatalHook) {
		t.Fatalf("expected fatal hook error, got %v", err)
	}
	reg = hook.NewRegistry()
	reg.On(agent.EventOnLLMRequest, func(context.Context, hook.Event) error {
		return errors.New("observe failed")
	})
	a = agent.New(agent.Config{Provider: newMockProvider(textResponse("unused")), Hooks: reg})
	if result, err := a.Run(context.Background(), []chat.Message{chat.User("hi")}); err != nil || result == nil {
		t.Fatalf("non-fatal hook error should be observed only, result=%v err=%v", result, err)
	}
}

func TestMoreMemoryPolicies(t *testing.T) {
	if _, err := (agent.FailStrategy{}).Compact(context.Background(), nil, 0); !errors.Is(err, agent.ErrContextExceeded) {
		t.Fatalf("fail strategy err=%v", err)
	}
	msgs := []chat.Message{chat.System("sys"), chat.User("old"), chat.Assistant("recent")}
	p := newMockProvider(textResponse("summary"))
	got, err := (agent.SummarizeStrategy{Provider: p, KeepLast: 1}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("summary got %d", len(got))
	}
	got, err = (agent.SummarizeStrategy{KeepLast: 1}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("fallback got %d", len(got))
	}
}
