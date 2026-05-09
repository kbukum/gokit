package anthropic

import (
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
		want         []ai.ToolUseBlock
	}{
		{
			name:         "single tool call",
			nonStreaming: `{"id":"msg-1","model":"claude-sonnet-4","content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"NYC"}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`,
			streaming: []string{
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"NYC\"}"}}`,
				`{"type":"message_stop"}`,
			},
			want: []ai.ToolUseBlock{{ID: "toolu_1", Name: "get_weather", Input: map[string]any{"city": "NYC"}}},
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
			want: []ai.ToolUseBlock{{ID: "toolu_2", Name: "search", Input: map[string]any{"q": "x"}}, {ID: "toolu_3", Name: "lookup", Input: map[string]any{"id": float64(7)}}},
		},
		{
			name:         "empty args",
			nonStreaming: `{"id":"msg-3","model":"claude-sonnet-4","content":[{"type":"tool_use","id":"toolu_4","name":"ping","input":{}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`,
			streaming: []string{
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_4","name":"ping"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
				`{"type":"message_stop"}`,
			},
			want: []ai.ToolUseBlock{{ID: "toolu_4", Name: "ping", Input: map[string]any{}}},
		},
		{
			name:         "nested input",
			nonStreaming: `{"id":"msg-4","model":"claude-sonnet-4","content":[{"type":"tool_use","id":"toolu_5","name":"plan_trip","input":{"trip":{"city":"Paris","days":[1,2]},"prefs":{"food":true}}}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`,
			streaming: []string{
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_5","name":"plan_trip"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"trip\":{\"city\":\"Paris\",\"days\":[1,2]},\"prefs\":{\"food\":true}}"}}`,
				`{"type":"message_stop"}`,
			},
			want: []ai.ToolUseBlock{{ID: "toolu_5", Name: "plan_trip", Input: map[string]any{"trip": map[string]any{"city": "Paris", "days": []any{float64(1), float64(2)}}, "prefs": map[string]any{"food": true}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := d.ParseResponse([]byte(tt.nonStreaming))
			if err != nil {
				t.Fatalf("ParseResponse: %v", err)
			}
			if !reflect.DeepEqual(resp.Message.ToolCalls, tt.want) {
				t.Fatalf("non-streaming tool calls = %#v, want %#v", resp.Message.ToolCalls, tt.want)
			}
			got := assembleToolUseBlocks(t, d, tt.streaming)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("streaming tool calls = %#v, want %#v", got, tt.want)
			}
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
