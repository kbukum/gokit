package llm

import (
	"context"
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// StreamEvent type tests
// ---------------------------------------------------------------------------

func TestStreamEvent_ContentDelta(t *testing.T) {
	var e StreamEvent = ContentDelta{Index: 0, Text: "Hello"}
	cd, ok := e.(ContentDelta)
	if !ok {
		t.Fatal("expected ContentDelta type")
	}
	if cd.Text != "Hello" {
		t.Errorf("expected text 'Hello', got %q", cd.Text)
	}
	if cd.Index != 0 {
		t.Errorf("expected index 0, got %d", cd.Index)
	}
}

func TestStreamEvent_ToolCallDelta(t *testing.T) {
	var e StreamEvent = ToolCallDelta{
		Index:      0,
		ID:         "call_123",
		Name:       "get_weather",
		InputDelta: `{"city":`,
	}
	tcd, ok := e.(ToolCallDelta)
	if !ok {
		t.Fatal("expected ToolCallDelta type")
	}
	if tcd.Name != "get_weather" {
		t.Errorf("expected name 'get_weather', got %q", tcd.Name)
	}
	if tcd.InputDelta != `{"city":` {
		t.Errorf("unexpected input delta: %q", tcd.InputDelta)
	}
}

func TestStreamEvent_ThinkingDelta(t *testing.T) {
	var e StreamEvent = ThinkingDelta{Text: "Let me think about this..."}
	td, ok := e.(ThinkingDelta)
	if !ok {
		t.Fatal("expected ThinkingDelta type")
	}
	if td.Text != "Let me think about this..." {
		t.Errorf("unexpected text: %q", td.Text)
	}
}

func TestStreamEvent_UsageUpdate(t *testing.T) {
	var e StreamEvent = UsageUpdate{InputTokens: 100, OutputTokens: 50}
	uu, ok := e.(UsageUpdate)
	if !ok {
		t.Fatal("expected UsageUpdate type")
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
		Message:    Assistant("Done!"),
		Model:      "gpt-4o",
		StopReason: StopEndTurn,
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
	if se.Error() != "llm: unknown stream error" {
		t.Errorf("unexpected nil error message: %q", se.Error())
	}
}

func TestStreamEvent_TypeSwitch(t *testing.T) {
	events := []StreamEvent{
		MessageStart{ID: "msg_1", Model: "gpt-4o"},
		ContentDelta{Index: 0, Text: "Hello"},
		ContentDelta{Index: 0, Text: " world"},
		UsageUpdate{InputTokens: 10, OutputTokens: 5},
		MessageComplete{Response: CompletionResponse{
			Message: Assistant("Hello world"),
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
		case ContentDelta:
			text += e.Text
		case MessageComplete:
			gotComplete = true
		case UsageUpdate:
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
		SupportsTools:     true,
		SupportsVision:    true,
		SupportsThinking:  false,
		SupportsStreaming: true,
		MaxContextTokens:  128000,
		MaxOutputTokens:   4096,
		ModelID:           "gpt-4o",
	}

	if !caps.SupportsTools {
		t.Error("expected SupportsTools=true")
	}
	if !caps.SupportsVision {
		t.Error("expected SupportsVision=true")
	}
	if caps.SupportsThinking {
		t.Error("expected SupportsThinking=false")
	}
	if !caps.SupportsStreaming {
		t.Error("expected SupportsStreaming=true")
	}
	if caps.MaxContextTokens != 128000 {
		t.Errorf("expected MaxContextTokens=128000, got %d", caps.MaxContextTokens)
	}
	if caps.MaxOutputTokens != 4096 {
		t.Errorf("expected MaxOutputTokens=4096, got %d", caps.MaxOutputTokens)
	}
	if caps.ModelID != "gpt-4o" {
		t.Errorf("expected ModelID='gpt-4o', got %q", caps.ModelID)
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

func (m *mockProvider) Complete(_ context.Context, _ CompletionRequest) (*CompletionResponse, error) {
	return m.resp, m.err
}

func (m *mockProvider) Stream(_ context.Context, _ CompletionRequest) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 3)
	ch <- MessageStart{ID: "msg_test", Model: m.caps.ModelID}
	ch <- ContentDelta{Index: 0, Text: m.resp.Text()}
	ch <- MessageComplete{Response: *m.resp}
	close(ch)
	return ch, nil
}

func (m *mockProvider) Capabilities() Capabilities {
	return m.caps
}

func (m *mockProvider) CountTokens(messages []Message) int {
	return CountTokensApprox(messages)
}

var _ Provider = (*mockProvider)(nil)

func TestProvider_Complete(t *testing.T) {
	p := &mockProvider{
		caps: Capabilities{ModelID: "test-model"},
		resp: &CompletionResponse{
			Message: Assistant("hello"),
			Model:   "test-model",
		},
	}

	resp, err := p.Complete(context.Background(), CompletionRequest{})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text() != "hello" {
		t.Errorf("expected text 'hello', got %q", resp.Text())
	}
}

func TestProvider_Stream(t *testing.T) {
	p := &mockProvider{
		caps: Capabilities{ModelID: "test-model"},
		resp: &CompletionResponse{
			Message: Assistant("streamed text"),
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

	// Second should be ContentDelta
	cd, ok := events[1].(ContentDelta)
	if !ok {
		t.Errorf("expected ContentDelta, got %T", events[1])
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
	count := CountTokensApprox(nil)
	if count != 0 {
		t.Errorf("expected 0 for nil messages, got %d", count)
	}
}

func TestCountTokensApprox_UserMessage(t *testing.T) {
	msgs := []Message{User("Hello world")} // 11 chars → ~3 tokens + 1 + 4 overhead = 8
	count := CountTokensApprox(msgs)
	if count < 5 || count > 15 {
		t.Errorf("expected roughly 5-15 tokens for 'Hello world', got %d", count)
	}
}

func TestCountTokensApprox_MultipleMessages(t *testing.T) {
	msgs := []Message{
		System("You are helpful"),
		User("What is 2+2?"),
		Assistant("4"),
	}
	count := CountTokensApprox(msgs)
	if count < 10 {
		t.Errorf("expected at least 10 tokens for conversation, got %d", count)
	}
}

func TestCountTokensApprox_WithToolCalls(t *testing.T) {
	msgs := []Message{
		AssistantMessage{
			ToolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"NYC"}`,
					},
				},
			},
		},
	}
	count := CountTokensApprox(msgs)
	if count < 5 {
		t.Errorf("expected at least 5 tokens for tool call, got %d", count)
	}
}

func TestCountTokensApprox_ToolResult(t *testing.T) {
	msgs := []Message{
		ToolResultMsg("call_1", "72°F and sunny in NYC", false),
	}
	count := CountTokensApprox(msgs)
	if count < 5 {
		t.Errorf("expected at least 5 tokens for tool result, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Enhanced Usage struct tests
// ---------------------------------------------------------------------------

func TestUsage_CacheFields(t *testing.T) {
	u := Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CacheReadTokens:  80,
		CacheWriteTokens: 20,
		ThinkingTokens:   30,
	}

	if u.CacheReadTokens != 80 {
		t.Errorf("CacheReadTokens = %d, want 80", u.CacheReadTokens)
	}
	if u.CacheWriteTokens != 20 {
		t.Errorf("CacheWriteTokens = %d, want 20", u.CacheWriteTokens)
	}
	if u.ThinkingTokens != 30 {
		t.Errorf("ThinkingTokens = %d, want 30", u.ThinkingTokens)
	}
}

// ---------------------------------------------------------------------------
// Enhanced CompletionRequest fields tests
// ---------------------------------------------------------------------------

func TestCompletionRequest_NewFields(t *testing.T) {
	topP := 0.9
	req := CompletionRequest{
		Model:         "gpt-4o",
		Messages:      []Message{User("test")},
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
