package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

func TestDialect_BuildRequestEncodesAssistantToolsChoicesStreamAndExtra(t *testing.T) {
	d := &Dialect{}
	temp := 0.3
	req := llm.CompletionRequest{
		Model:       "claude-test",
		Stream:      true,
		Temperature: &temp,
		Messages: []chat.Message{
			chat.System("system as message"),
			chat.AssistantMessage{
				Content: ai.TextContent("using tool"),
				ToolCalls: []ai.ToolUseBlock{{
					ID:    "toolu_1",
					Name:  "lookup",
					Input: json.RawMessage(`{"id":7}`),
				}},
			},
		},
		Tools: []ai.ToolSpec{{
			Name:        "lookup",
			Description: "Lookup by id",
			InputSchema: map[string]any{"type": "object"},
		}},
		ToolChoice: llm.ToolChoiceFunc("lookup"),
		Extra:      llm.RawJSON(`{"metadata":{"trace":"abc"}}`),
	}
	body, err := d.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest: %v", err)
	}
	got := mustJSONMap(t, body)
	if got["stream"] != true || got["temperature"] != 0.3 {
		t.Fatalf("options = %#v", got)
	}
	messages := got["messages"].([]any)
	if messages[0].(map[string]any)["role"] != "user" {
		t.Fatalf("system message = %#v", messages[0])
	}
	assistant := messages[1].(map[string]any)
	blocks := assistant["content"].([]any)
	if blocks[0].(map[string]any)["type"] != "text" || blocks[1].(map[string]any)["type"] != "tool_use" {
		t.Fatalf("assistant blocks = %#v", blocks)
	}
	choice := got["tool_choice"].(map[string]any)
	if choice["type"] != "tool" || choice["name"] != "lookup" {
		t.Fatalf("tool choice = %#v", choice)
	}
	tools := got["tools"].([]any)
	if tools[0].(map[string]any)["input_schema"] == nil {
		t.Fatalf("tools = %#v", tools)
	}
	metadata := got["metadata"].(map[string]any)
	if metadata["trace"] != "abc" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestDialect_BuildRequestToolChoiceModesAndInvalidExtra(t *testing.T) {
	d := &Dialect{}
	for _, tt := range []struct {
		choice *llm.ToolChoice
		want   any
	}{
		{choice: llm.ToolChoiceAuto, want: "auto"},
		{choice: llm.ToolChoiceNone, want: nil},
		{choice: &llm.ToolChoice{Mode: "unexpected"}, want: "auto"},
	} {
		body, err := d.BuildRequest(llm.CompletionRequest{Model: "claude-test", Messages: []chat.Message{chat.User("hi")}, ToolChoice: tt.choice})
		if err != nil {
			t.Fatalf("BuildRequest: %v", err)
		}
		got := mustJSONMap(t, body)
		if tt.want == nil {
			if got["tool_choice"] != nil {
				t.Fatalf("tool_choice = %#v, want nil", got["tool_choice"])
			}
			continue
		}
		choice := got["tool_choice"].(map[string]any)
		if choice["type"] != tt.want {
			t.Fatalf("tool_choice = %#v, want %v", choice, tt.want)
		}
	}
	_, err := d.BuildRequest(llm.CompletionRequest{Model: "claude-test", Extra: llm.RawJSON(`{"bad"`)})
	if err == nil {
		t.Fatal("expected invalid extra error")
	}
}

func TestDialect_ParseResponseMalformedAndStopReasons(t *testing.T) {
	d := &Dialect{}
	for _, tt := range []struct {
		name       string
		stop       string
		wantReason chat.FinishReason
	}{
		{name: "max tokens", stop: "max_tokens", wantReason: chat.FinishReasonLength},
		{name: "stop sequence", stop: "stop_sequence", wantReason: chat.FinishReasonStop},
		{name: "unknown", stop: "unknown", wantReason: chat.FinishReasonStop},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := `{"model":"claude-test","content":[{"type":"unknown"},{"type":"text","text":"ok"}],"stop_reason":"` + tt.stop + `","usage":{"input_tokens":1,"output_tokens":2}}`
			resp, err := d.ParseResponse([]byte(raw))
			if err != nil {
				t.Fatalf("ParseResponse: %v", err)
			}
			if resp.Text() != "ok" || resp.StopReason != tt.wantReason {
				t.Fatalf("response text=%q reason=%v", resp.Text(), resp.StopReason)
			}
		})
	}
	if _, err := d.ParseResponse([]byte(`{`)); err == nil {
		t.Fatal("expected malformed response error")
	}
}

func TestDialect_ParseStreamChunkMalformedAndIgnoredEvents(t *testing.T) {
	d := &Dialect{}
	for _, raw := range []string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"unknown"}}`,
		`{"type":"message_delta"}`,
	} {
		chunk, err := d.ParseStreamChunk([]byte(raw))
		if err != nil {
			t.Fatalf("ParseStreamChunk(%s): %v", raw, err)
		}
		if chunk.Content != "" || len(chunk.ToolCalls) != 0 || chunk.Done {
			t.Fatalf("chunk = %#v", chunk)
		}
	}
	if _, err := d.ParseStreamChunk([]byte(`{`)); err == nil {
		t.Fatal("expected malformed stream error")
	}
}

func FuzzDialectJSONCodecs(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`),
		[]byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"ok"}}`),
		[]byte(`{"type":"message_stop"}`),
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
