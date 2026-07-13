package anthropic

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/llm/internal/streamwire"
)

func TestDialect_ToolUseFixtures(t *testing.T) {
	d := &Dialect{}
	tests := []struct {
		name         string
		nonStreaming string
		streaming    []string
		want         []toolCallWant
	}{
		{
			name:         "single tool call",
			nonStreaming: `{"id":"msg-1","model":"claude-sonnet-4","content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"NYC"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`,
			streaming: []string{
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"NYC\"}"}}`,
				`{"type":"message_stop"}`,
			},
			want: []toolCallWant{{ID: "toolu_1", Name: "get_weather", Input: map[string]any{"city": "NYC"}}},
		},
		{
			name:         "multi tool response",
			nonStreaming: `{"id":"msg-2","model":"claude-sonnet-4","content":[{"type":"tool_use","id":"toolu_2","name":"search","input":{"q":"x"}},{"type":"tool_use","id":"toolu_3","name":"lookup","input":{"id":7}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`,
			streaming: []string{
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_2","name":"search"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"q\":\"x\"}"}}`,
				`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_3","name":"lookup"}}`,
				`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"id\":7}"}}`,
				`{"type":"message_stop"}`,
			},
			want: []toolCallWant{{ID: "toolu_2", Name: "search", Input: map[string]any{"q": "x"}}, {ID: "toolu_3", Name: "lookup", Input: map[string]any{"id": float64(7)}}},
		},
		{
			name:         "empty args",
			nonStreaming: `{"id":"msg-3","model":"claude-sonnet-4","content":[{"type":"tool_use","id":"toolu_4","name":"ping","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`,
			streaming: []string{
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_4","name":"ping"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
				`{"type":"message_stop"}`,
			},
			want: []toolCallWant{{ID: "toolu_4", Name: "ping", Input: map[string]any{}}},
		},
		{
			name:         "nested input",
			nonStreaming: `{"id":"msg-4","model":"claude-sonnet-4","content":[{"type":"tool_use","id":"toolu_5","name":"plan_trip","input":{"trip":{"city":"Paris","days":[1,2]},"prefs":{"food":true}}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`,
			streaming: []string{
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_5","name":"plan_trip"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"trip\":{\"city\":\"Paris\",\"days\":[1,2]},\"prefs\":{\"food\":true}}"}}`,
				`{"type":"message_stop"}`,
			},
			want: []toolCallWant{{ID: "toolu_5", Name: "plan_trip", Input: map[string]any{"trip": map[string]any{"city": "Paris", "days": []any{float64(1), float64(2)}}, "prefs": map[string]any{"food": true}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := d.ParseResponse([]byte(tt.nonStreaming))
			if err != nil {
				t.Fatalf("ParseResponse: %v", err)
			}
			assertToolCalls(t, resp.Message.ToolCalls, tt.want)
			got := assembleToolUseBlocks(t, d, tt.streaming)
			assertToolCalls(t, got, tt.want)
		})
	}
}

func assembleToolUseBlocks(t *testing.T, d *Dialect, events []string) []ai.ToolUseBlock {
	t.Helper()
	var calls []streamwire.ToolCall
	for _, raw := range events {
		chunk, err := d.ParseStreamChunk([]byte(raw))
		if err != nil {
			t.Fatalf("ParseStreamChunk: %v", err)
		}
		for _, tc := range chunk.ToolCalls {
			calls = streamwire.MergeToolDelta(calls, tc)
		}
	}
	blocks, err := streamwire.ToolUseBlocks(calls)
	if err != nil {
		t.Fatalf("ToolUseBlocks: %v", err)
	}
	return blocks
}

// toolCallWant is the readable expectation form for a decoded tool call: the
// arguments are compared by JSON value (not raw bytes) so key ordering and
// whitespace differences between streaming and non-streaming paths do not
// matter.
type toolCallWant struct {
	ID    string
	Name  string
	Input map[string]any
}

func assertToolCalls(t *testing.T, got []ai.ToolUseBlock, want []toolCallWant) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("tool calls = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].ID != want[i].ID || got[i].Name != want[i].Name {
			t.Fatalf("tool[%d] = %+v, want %+v", i, got[i], want[i])
		}
		gm := map[string]any{}
		if len(got[i].Input) > 0 {
			if err := json.Unmarshal(got[i].Input, &gm); err != nil {
				t.Fatalf("tool[%d] input not a JSON object: %v", i, err)
			}
		}
		w := want[i].Input
		if w == nil {
			w = map[string]any{}
		}
		if !reflect.DeepEqual(gm, w) {
			t.Fatalf("tool[%d] input = %#v, want %#v", i, gm, w)
		}
	}
}
