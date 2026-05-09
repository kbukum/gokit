package openai

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
			nonStreaming: `{"id":"chatcmpl-1","model":"gpt-4o","choices":[{"message":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"NYC\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			streaming: []string{
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"arguments":"\"NYC\"}"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			},
			want: []ai.ToolUseBlock{{ID: "call_1", Name: "get_weather", Input: map[string]any{"city": "NYC"}}},
		},
		{
			name:         "multi tool response",
			nonStreaming: `{"id":"chatcmpl-2","model":"gpt-4o","choices":[{"message":{"tool_calls":[{"id":"call_2","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}},{"id":"call_3","type":"function","function":{"name":"lookup","arguments":"{\"id\":7}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			streaming: []string{
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"search","arguments":"{\"q\":\"x\"}"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_3","type":"function","function":{"name":"lookup","arguments":"{\"id\":7}"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			},
			want: []ai.ToolUseBlock{{ID: "call_2", Name: "search", Input: map[string]any{"q": "x"}}, {ID: "call_3", Name: "lookup", Input: map[string]any{"id": float64(7)}}},
		},
		{
			name:         "empty args",
			nonStreaming: `{"id":"chatcmpl-3","model":"gpt-4o","choices":[{"message":{"tool_calls":[{"id":"call_4","type":"function","function":{"name":"ping","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			streaming: []string{
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_4","type":"function","function":{"name":"ping","arguments":"{}"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			},
			want: []ai.ToolUseBlock{{ID: "call_4", Name: "ping", Input: map[string]any{}}},
		},
		{
			name:         "nested input",
			nonStreaming: `{"id":"chatcmpl-4","model":"gpt-4o","choices":[{"message":{"tool_calls":[{"id":"call_5","type":"function","function":{"name":"plan_trip","arguments":"{\"trip\":{\"city\":\"Paris\",\"days\":[1,2]},\"prefs\":{\"food\":true}}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
			streaming: []string{
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_5","type":"function","function":{"name":"plan_trip","arguments":"{\"trip\":{\"city\":\"Paris\",\"days\":[1,2]},\"prefs\":{\"food\":true}}"}}]},"finish_reason":null}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			},
			want: []ai.ToolUseBlock{{ID: "call_5", Name: "plan_trip", Input: map[string]any{"trip": map[string]any{"city": "Paris", "days": []any{float64(1), float64(2)}}, "prefs": map[string]any{"food": true}}}},
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
