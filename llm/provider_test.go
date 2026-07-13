package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/ai/chat"

	"github.com/kbukum/gokit/ai"
)

// ---------------------------------------------------------------------------
// StreamEvent type tests
// ---------------------------------------------------------------------------

func TestStreamEvent_TextDelta(t *testing.T) {
	var e StreamEvent = TextDelta{Index: 0, Text: "Hello"}
	cd, ok := e.(TextDelta)
	if !ok {
		t.Fatal("expected TextDelta type")
	}
	if cd.Text != "Hello" {
		t.Errorf("expected text 'Hello', got %q", cd.Text)
	}
	if cd.Index != 0 {
		t.Errorf("expected index 0, got %d", cd.Index)
	}
}

func TestStreamEvent_ToolUseDelta(t *testing.T) {
	var e StreamEvent = ToolUseDelta{
		Index:      0,
		ID:         "call_123",
		Name:       "get_weather",
		InputDelta: `{"city":`,
	}
	tcd, ok := e.(ToolUseDelta)
	if !ok {
		t.Fatal("expected ToolUseDelta type")
	}
	if tcd.Name != "get_weather" {
		t.Errorf("expected name 'get_weather', got %q", tcd.Name)
	}
	if tcd.InputDelta != `{"city":` {
		t.Errorf("unexpected input delta: %q", tcd.InputDelta)
	}
}

func TestStreamEvent_ReasoningDelta(t *testing.T) {
	var e StreamEvent = ReasoningDelta{Text: "Let me think about this..."}
	td, ok := e.(ReasoningDelta)
	if !ok {
		t.Fatal("expected ReasoningDelta type")
	}
	if td.Text != "Let me think about this..." {
		t.Errorf("unexpected text: %q", td.Text)
	}
}

func TestStreamEvent_UsageDelta(t *testing.T) {
	var e StreamEvent = UsageDelta{InputTokens: 100, OutputTokens: 50}
	uu, ok := e.(UsageDelta)
	if !ok {
		t.Fatal("expected UsageDelta type")
	}
	if uu.InputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", uu.InputTokens)
	}
}

func TestStreamEvent_MessageStart(t *testing.T) {
	var e StreamEvent = MessageStart{ID: "msg_123", Model: "gpt-4o"}
	ms, ok := e.(MessageStart)
	if !ok {
		t.Fatal("expected MessageStart type")
	}
	if ms.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", ms.Model)
	}
}

func TestStreamEvent_MessageComplete(t *testing.T) {
	resp := CompletionResponse{
		Message:    chat.Assistant("Done!"),
		Model:      "gpt-4o",
		StopReason: chat.FinishReasonStop,
	}
	var e StreamEvent = MessageComplete{Response: resp}
	mc, ok := e.(MessageComplete)
	if !ok {
		t.Fatal("expected MessageComplete type")
	}
	if mc.Response.Text() != "Done!" {
		t.Errorf("expected text 'Done!', got %q", mc.Response.Text())
	}
}

func TestStreamEvent_Error(t *testing.T) {
	var e StreamEvent = StreamError{Err: fmt.Errorf("connection lost")}
	se, ok := e.(StreamError)
	if !ok {
		t.Fatal("expected StreamError type")
	}
	if se.Error() != "connection lost" {
		t.Errorf("unexpected error message: %q", se.Error())
	}
}

func TestStreamEvent_ErrorNil(t *testing.T) {
	se := StreamError{}
	if se.Error() != "ai: unknown stream error" {
		t.Errorf("unexpected nil error message: %q", se.Error())
	}
}

func TestStreamEvent_TypeSwitch(t *testing.T) {
	events := []StreamEvent{
		MessageStart{ID: "msg_1", Model: "gpt-4o"},
		TextDelta{Index: 0, Text: "Hello"},
		TextDelta{Index: 0, Text: " world"},
		UsageDelta{InputTokens: 10, OutputTokens: 5},
		MessageComplete{Response: CompletionResponse{
			Message: chat.Assistant("Hello world"),
			Model:   "gpt-4o",
		}},
	}

	var text string
	var gotStart, gotComplete bool

	for _, event := range events {
		switch e := event.(type) {
		case MessageStart:
			gotStart = true
			if e.ID != "msg_1" {
				t.Errorf("unexpected message ID: %q", e.ID)
			}
		case TextDelta:
			text += e.Text
		case MessageComplete:
			gotComplete = true
		case UsageDelta:
			// expected
		default:
			t.Errorf("unexpected event type: %T", e)
		}
	}

	if text != "Hello world" {
		t.Errorf("accumulated text = %q, want 'Hello world'", text)
	}
	if !gotStart {
		t.Error("expected MessageStart event")
	}
	if !gotComplete {
		t.Error("expected MessageComplete event")
	}
}

// ---------------------------------------------------------------------------
// Capabilities tests
// ---------------------------------------------------------------------------

func TestCapabilities_Fields(t *testing.T) {
	caps := Capabilities{
		ToolUse:         true,
		Vision:          true,
		ReasoningTokens: false,
		Streaming:       true,
		MaxInputTokens:  128000,
		MaxOutputTokens: 4096,
	}

	if !caps.ToolUse {
		t.Error("expected ToolUse=true")
	}
	if !caps.Vision {
		t.Error("expected Vision=true")
	}
	if caps.ReasoningTokens {
		t.Error("expected ReasoningTokens=false")
	}
	if !caps.Streaming {
		t.Error("expected Streaming=true")
	}
	if caps.MaxInputTokens != 128000 {
		t.Errorf("expected MaxInputTokens=128000, got %d", caps.MaxInputTokens)
	}
	if caps.MaxOutputTokens != 4096 {
		t.Errorf("expected MaxOutputTokens=4096, got %d", caps.MaxOutputTokens)
	}
	if caps.MaxOutputTokens != 4096 {
		t.Errorf("expected MaxOutputTokens=4096, got %d", caps.MaxOutputTokens)
	}
}

// ---------------------------------------------------------------------------
// Provider interface compliance test
// ---------------------------------------------------------------------------

// mockProvider implements Provider for testing.
type mockProvider struct {
	caps Capabilities
	resp *CompletionResponse
	err  error
}

func (m *mockProvider) Name() string                       { return "mock" }
func (m *mockProvider) IsAvailable(_ context.Context) bool { return true }
func (m *mockProvider) Execute(_ context.Context, _ CompletionRequest) (CompletionResponse, error) {
	if m.resp == nil {
		return CompletionResponse{}, m.err
	}
	return *m.resp, m.err
}

func (m *mockProvider) Stream(_ context.Context, _ CompletionRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 3)
	ch <- MessageStart{ID: "msg_test", Model: "model"}
	ch <- TextDelta{Index: 0, Text: m.resp.Text()}
	ch <- MessageComplete{Response: *m.resp}
	close(ch)
	return ch, nil
}

func (m *mockProvider) Capabilities() Capabilities {
	return m.caps
}

func (m *mockProvider) CountTokens(messages []chat.Message) int {
	return chat.CountTokensApprox(messages)
}

var _ Provider = (*mockProvider)(nil)

func TestProvider_Complete(t *testing.T) {
	p := &mockProvider{
		caps: Capabilities{Streaming: true, MaxInputTokens: 128000},
		resp: &CompletionResponse{
			Message: chat.Assistant("hello"),
			Model:   "test-model",
		},
	}

	resp, err := p.Execute(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Text() != "hello" {
		t.Errorf("expected text 'hello', got %q", resp.Text())
	}
}

func TestProvider_Stream(t *testing.T) {
	p := &mockProvider{
		caps: Capabilities{Streaming: true, MaxInputTokens: 128000},
		resp: &CompletionResponse{
			Message: chat.Assistant("streamed text"),
			Model:   "test-model",
		},
	}

	ch, err := p.Stream(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	events := make([]StreamEvent, 0, 3)
	for e := range ch {
		events = append(events, e)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// First event should be MessageStart
	if _, ok := events[0].(MessageStart); !ok {
		t.Errorf("expected MessageStart, got %T", events[0])
	}

	// Second should be TextDelta
	cd, ok := events[1].(TextDelta)
	if !ok {
		t.Errorf("expected TextDelta, got %T", events[1])
	}
	if cd.Text != "streamed text" {
		t.Errorf("expected 'streamed text', got %q", cd.Text)
	}

	// Third should be MessageComplete
	if _, ok := events[2].(MessageComplete); !ok {
		t.Errorf("expected MessageComplete, got %T", events[2])
	}
}

// ---------------------------------------------------------------------------
// CountTokensApprox tests
// ---------------------------------------------------------------------------

func TestCountTokensApprox_Empty(t *testing.T) {
	count := chat.CountTokensApprox(nil)
	if count != 0 {
		t.Errorf("expected 0 for nil messages, got %d", count)
	}
}

func TestCountTokensApproxUserMessage(t *testing.T) {
	msgs := []chat.Message{chat.User("Hello world")} // 11 chars → ~3 tokens + 1 + 4 overhead = 8
	count := chat.CountTokensApprox(msgs)
	if count < 5 || count > 15 {
		t.Errorf("expected roughly 5-15 tokens for 'Hello world', got %d", count)
	}
}

func TestCountTokensApprox_MultipleMessages(t *testing.T) {
	msgs := []chat.Message{
		chat.System("You are helpful"),
		chat.User("What is 2+2?"),
		chat.Assistant("4"),
	}
	count := chat.CountTokensApprox(msgs)
	if count < 10 {
		t.Errorf("expected at least 10 tokens for conversation, got %d", count)
	}
}

func TestCountTokensApprox_WithToolCalls(t *testing.T) {
	msgs := []chat.Message{
		chat.AssistantMessage{
			ToolCalls: []ai.ToolUseBlock{{ID: "call_1", Name: "get_weather", Input: json.RawMessage(`{"city":"NYC"}`)}},
		},
	}
	count := chat.CountTokensApprox(msgs)
	if count < 5 {
		t.Errorf("expected at least 5 tokens for tool call, got %d", count)
	}
}

func TestCountTokensApprox_ToolResult(t *testing.T) {
	msgs := []chat.Message{
		chat.ToolResultMsg("call_1", "72°F and sunny in NYC", false),
	}
	count := chat.CountTokensApprox(msgs)
	if count < 5 {
		t.Errorf("expected at least 5 tokens for tool result, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Enhanced Usage struct tests
// ---------------------------------------------------------------------------

func TestUsage_CacheFields(t *testing.T) {
	u := Usage{
		InputTokens:     100,
		OutputTokens:    50,
		CachedTokens:    80,
		ReasoningTokens: 30,
	}

	if u.CachedTokens != 80 {
		t.Errorf("CachedTokens = %d, want 80", u.CachedTokens)
	}
	if u.ReasoningTokens != 30 {
		t.Errorf("ReasoningTokens = %d, want 30", u.ReasoningTokens)
	}
}

// ---------------------------------------------------------------------------
// Enhanced CompletionRequest fields tests
// ---------------------------------------------------------------------------

func TestCompletionRequest_NewFields(t *testing.T) {
	topP := 0.9
	req := CompletionRequest{
		Model:         "gpt-4o",
		Messages:      []chat.Message{chat.User("test")},
		TopP:          &topP,
		StopSequences: []string{"END", "STOP"},
		Metadata:      map[string]string{"request_id": "abc123"},
	}

	if *req.TopP != 0.9 {
		t.Errorf("TopP = %v, want 0.9", *req.TopP)
	}
	if len(req.StopSequences) != 2 {
		t.Errorf("StopSequences length = %d, want 2", len(req.StopSequences))
	}
	if req.Metadata["request_id"] != "abc123" {
		t.Errorf("Metadata[request_id] = %q, want 'abc123'", req.Metadata["request_id"])
	}
}
