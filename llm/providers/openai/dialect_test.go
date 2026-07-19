package openai

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

func TestDialect_BuildRequestEncodesMessageKindsToolsAndExtra(t *testing.T) {
	d := &Dialect{}
	temp := 0.2
	req := llm.CompletionRequest{
		Model:       "gpt-test",
		Temperature: &temp,
		MaxTokens:   64,
		Messages: []chat.Message{
			chat.System("inline system"),
			chat.User("question"),
			chat.AssistantMessage{
				Content: ai.TextContent("using a tool"),
				ToolCalls: []ai.ToolUseBlock{{
					ID:    "call_1",
					Name:  "lookup",
					Input: json.RawMessage(`{"id":7}`),
				}},
			},
			chat.ToolResultMsg("call_1", "result", false),
		},
		Tools: []ai.ToolSpec{{
			Name:        "lookup",
			Description: "Lookup by id",
			InputSchema: map[string]any{"type": "object"},
		}},
		ToolChoice: llm.ToolChoiceFunc("lookup"),
		Extra:      llm.RawJSON(`{"parallel_tool_calls":false}`),
	}

	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	got := mustJSONMap(t, body)
	if got["temperature"] != 0.2 || got["max_tokens"] != float64(64) || got["parallel_tool_calls"] != false {
		t.Fatalf("options not encoded: %#v", got)
	}
	messages := got["messages"].([]any)
	roles := []string{"system", "user", "assistant", "tool"}
	for i, role := range roles {
		msg := messages[i].(map[string]any)
		if msg["role"] != role {
			t.Fatalf("message[%d] role = %v, want %s", i, msg["role"], role)
		}
	}
	assistant := messages[2].(map[string]any)
	toolCalls := assistant["tool_calls"].([]any)
	toolCall := toolCalls[0].(map[string]any)
	if toolCall["id"] != "call_1" {
		t.Fatalf("assistant tool call = %#v", toolCall)
	}
	toolChoice := got["tool_choice"].(map[string]any)
	fn := toolChoice["function"].(map[string]any)
	if fn["name"] != "lookup" {
		t.Fatalf("tool_choice = %#v", toolChoice)
	}
	tools := got["tools"].([]any)
	if tools[0].(map[string]any)["type"] != "function" {
		t.Fatalf("tools = %#v", tools)
	}
}

func TestDialect_BuildRequestToolChoiceModesAndInvalidExtra(t *testing.T) {
	d := &Dialect{}
	for _, tc := range []*llm.ToolChoice{llm.ToolChoiceAuto, llm.ToolChoiceNone, {Mode: "unexpected"}} {
		body, err := d.BuildRequest(llm.CompletionRequest{Model: "gpt-test", Messages: []chat.Message{chat.User("hi")}, ToolChoice: tc})
		if err != nil {
			t.Fatalf("BuildRequest(%s): %v", tc.Mode, err)
		}
		got := mustJSONMap(t, body)
		want := tc.Mode
		if want == "unexpected" {
			want = "auto"
		}
		if got["tool_choice"] != want {
			t.Fatalf("tool_choice = %v, want %s", got["tool_choice"], want)
		}
	}
	_, err := d.BuildRequest(llm.CompletionRequest{Model: "gpt-test", Extra: llm.RawJSON(`{"bad"`)})
	if err == nil {
		t.Fatal("expected invalid extra error")
	}
}

func TestDialect_ParseResponseReasoningContentAndFinishReasons(t *testing.T) {
	d := &Dialect{}
	for _, tt := range []struct {
		name       string
		finish     string
		wantReason chat.FinishReason
	}{
		{name: "length", finish: "length", wantReason: chat.FinishReasonLength},
		{name: "content filter", finish: "content_filter", wantReason: chat.FinishReasonContentFilter},
		{name: "unknown", finish: "unknown", wantReason: chat.FinishReasonStop},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := `{"model":"gpt-test","choices":[{"message":{"content":"","reasoning_content":"thinking"},"finish_reason":"` + tt.finish + `"}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`
			resp, err := d.ParseResponse([]byte(raw))
			if err != nil {
				t.Fatalf("ParseResponse: %v", err)
			}
			if resp.Text() != "thinking" || resp.StopReason != tt.wantReason {
				t.Fatalf("response = text %q reason %v", resp.Text(), resp.StopReason)
			}
		})
	}
	if _, err := d.ParseResponse([]byte(`{"choices":[]}`)); err == nil {
		t.Fatal("expected no choices error")
	}
	if _, err := d.ParseResponse([]byte(`{`)); err == nil {
		t.Fatal("expected malformed response error")
	}
}

func TestDialect_ParseStreamChunkReasoningEmptyAndMalformed(t *testing.T) {
	d := &Dialect{}
	chunk, err := d.ParseStreamChunk([]byte(`{"choices":[{"delta":{"reasoning_content":"think"}}]}`))
	if err != nil {
		t.Fatalf("ParseStreamChunk reasoning: %v", err)
	}
	if chunk.Content != "think" {
		t.Fatalf("content = %q", chunk.Content)
	}
	chunk, err = d.ParseStreamChunk([]byte(`{"choices":[]}`))
	if err != nil {
		t.Fatalf("ParseStreamChunk empty: %v", err)
	}
	if chunk.Content != "" || chunk.Done {
		t.Fatalf("empty chunk = %#v", chunk)
	}
	if _, err := d.ParseStreamChunk([]byte(`{`)); err == nil {
		t.Fatal("expected malformed stream error")
	}
}

func FuzzDialectJSONCodecs(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`),
		[]byte(`{"choices":[{"delta":{"content":"ok"}}]}`),
		[]byte(`[DONE]`),
		[]byte(`{`),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	d := &Dialect{}
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = d.ParseResponse(data)
		_, _ = d.ParseStreamChunk(data)
	})
}

func mustJSONMap(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	return got
}
