package gemini

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
			nonStreaming: `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"NYC"}}}],"role":"model"},"finishReason":"TOOL_USE"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
			streaming: []string{
				`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"get_weather","args":{"city":"NYC"}}}],"role":"model"},"finishReason":""}]}`,
				`{"candidates":[{"content":{"parts":[]},"finishReason":"TOOL_USE"}]}`,
			},
			want: []ai.ToolUseBlock{{ID: "call_0", Name: "get_weather", Input: map[string]any{"city": "NYC"}}},
		},
		{
			name:         "multi tool response",
			nonStreaming: `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"search","args":{"q":"x"}}},{"functionCall":{"name":"lookup","args":{"id":7}}}],"role":"model"},"finishReason":"TOOL_USE"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
			streaming: []string{
				`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"search","args":{"q":"x"}}},{"functionCall":{"name":"lookup","args":{"id":7}}}],"role":"model"},"finishReason":""}]}`,
				`{"candidates":[{"content":{"parts":[]},"finishReason":"TOOL_USE"}]}`,
			},
			want: []ai.ToolUseBlock{{ID: "call_0", Name: "search", Input: map[string]any{"q": "x"}}, {ID: "call_1", Name: "lookup", Input: map[string]any{"id": float64(7)}}},
		},
		{
			name:         "empty args",
			nonStreaming: `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"ping","args":{}}}],"role":"model"},"finishReason":"TOOL_USE"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
			streaming: []string{
				`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"ping","args":{}}}],"role":"model"},"finishReason":""}]}`,
				`{"candidates":[{"content":{"parts":[]},"finishReason":"TOOL_USE"}]}`,
			},
			want: []ai.ToolUseBlock{{ID: "call_0", Name: "ping", Input: map[string]any{}}},
		},
		{
			name:         "nested input",
			nonStreaming: `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"plan_trip","args":{"trip":{"city":"Paris","days":[1,2]},"prefs":{"food":true}}}}],"role":"model"},"finishReason":"TOOL_USE"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`,
			streaming: []string{
				`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"plan_trip","args":{"trip":{"city":"Paris","days":[1,2]},"prefs":{"food":true}}}}],"role":"model"},"finishReason":""}]}`,
				`{"candidates":[{"content":{"parts":[]},"finishReason":"TOOL_USE"}]}`,
			},
			want: []ai.ToolUseBlock{{ID: "call_0", Name: "plan_trip", Input: map[string]any{"trip": map[string]any{"city": "Paris", "days": []any{float64(1), float64(2)}}, "prefs": map[string]any{"food": true}}}},
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
