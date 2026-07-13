package streamwire

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeToolDelta(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		calls []ToolCall
		delta ToolCall
		want  []ToolCall
	}{
		{
			name:  "first call appended by id",
			calls: nil,
			delta: ToolCall{Index: 0, ID: "a", Name: "search", InputDelta: `{"q":`},
			want:  []ToolCall{{Index: 0, ID: "a", Name: "search", InputDelta: `{"q":`}},
		},
		{
			name:  "same id merges name index and input",
			calls: []ToolCall{{Index: 0, ID: "a", Name: "search", InputDelta: `{"q":`}},
			delta: ToolCall{Index: 1, ID: "a", Name: "search2", InputDelta: `"x"}`},
			want:  []ToolCall{{Index: 1, ID: "a", Name: "search2", InputDelta: `{"q":"x"}`}},
		},
		{
			name:  "new id appended",
			calls: []ToolCall{{Index: 0, ID: "a", InputDelta: `{}`}},
			delta: ToolCall{Index: 1, ID: "b", Name: "other", InputDelta: `{`},
			want: []ToolCall{
				{Index: 0, ID: "a", InputDelta: `{}`},
				{Index: 1, ID: "b", Name: "other", InputDelta: `{`},
			},
		},
		{
			name:  "match by index when no id",
			calls: []ToolCall{{Index: 2, Name: "t", InputDelta: `{"a":`}},
			delta: ToolCall{Index: 2, InputDelta: `1}`},
			want:  []ToolCall{{Index: 2, Name: "t", InputDelta: `{"a":1}`}},
		},
		{
			name:  "index not found appended",
			calls: []ToolCall{{Index: 0, Name: "t", InputDelta: `{}`}},
			delta: ToolCall{Index: 5, Name: "u", InputDelta: `{`},
			want: []ToolCall{
				{Index: 0, Name: "t", InputDelta: `{}`},
				{Index: 5, Name: "u", InputDelta: `{`},
			},
		},
		{
			name:  "match by name when no id or index",
			calls: []ToolCall{{Index: -1, Name: "calc", InputDelta: `{"x":`}},
			delta: ToolCall{Index: -1, Name: "calc", InputDelta: `2}`},
			want:  []ToolCall{{Index: -1, Name: "calc", InputDelta: `{"x":2}`}},
		},
		{
			name:  "name not found appended",
			calls: []ToolCall{{Index: -1, Name: "calc", InputDelta: `{}`}},
			delta: ToolCall{Index: -1, Name: "other", InputDelta: `{`},
			want: []ToolCall{
				{Index: -1, Name: "calc", InputDelta: `{}`},
				{Index: -1, Name: "other", InputDelta: `{`},
			},
		},
		{
			name:  "bare delta folds into last call",
			calls: []ToolCall{{Index: -1, Name: "calc", InputDelta: `{"x":`}},
			delta: ToolCall{Index: -1, InputDelta: `3}`},
			want:  []ToolCall{{Index: -1, Name: "calc", InputDelta: `{"x":3}`}},
		},
		{
			name:  "bare delta with no calls appended",
			calls: nil,
			delta: ToolCall{Index: -1, InputDelta: `{}`},
			want:  []ToolCall{{Index: -1, InputDelta: `{}`}},
		},
		{
			name:  "same id keeps index when delta index negative",
			calls: []ToolCall{{Index: 3, ID: "a", Name: "search", InputDelta: `{"q":`}},
			delta: ToolCall{Index: -1, ID: "a", InputDelta: `"x"}`},
			want:  []ToolCall{{Index: 3, ID: "a", Name: "search", InputDelta: `{"q":"x"}`}},
		},
		{
			name:  "same id keeps name when delta name empty",
			calls: []ToolCall{{Index: 0, ID: "a", Name: "search", InputDelta: `{`}},
			delta: ToolCall{Index: 0, ID: "a", InputDelta: `}`},
			want:  []ToolCall{{Index: 0, ID: "a", Name: "search", InputDelta: `{}`}},
		},
		{
			name:  "match by index keeps name when delta name empty",
			calls: []ToolCall{{Index: 2, Name: "t", InputDelta: `{`}},
			delta: ToolCall{Index: 2, InputDelta: `}`},
			want:  []ToolCall{{Index: 2, Name: "t", InputDelta: `{}`}},
		},
		{
			name:  "match by index sets name from delta",
			calls: []ToolCall{{Index: 2, InputDelta: `{`}},
			delta: ToolCall{Index: 2, Name: "late", InputDelta: `}`},
			want:  []ToolCall{{Index: 2, Name: "late", InputDelta: `{}`}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MergeToolDelta(tt.calls, tt.delta)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%+v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("call[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestToolUseBlocks(t *testing.T) {
	t.Parallel()
	blocks, err := ToolUseBlocks([]ToolCall{
		{ID: "a", Name: "search", InputDelta: `{"q":"x"}`},
		{ID: "b", Name: "empty", InputDelta: ``},
	})
	if err != nil {
		t.Fatalf("ToolUseBlocks() error: %v", err)
	}
	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if blocks[0].ID != "a" || string(blocks[0].Input) != `{"q":"x"}` {
		t.Errorf("block[0] = %+v", blocks[0])
	}
	if string(blocks[1].Input) != `{}` {
		t.Errorf("empty input normalized to %q, want {}", blocks[1].Input)
	}
}

func TestToolUseBlocksRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ToolUseBlocks([]ToolCall{{
		ID:         "call_1",
		Name:       "broken",
		InputDelta: `{"a":`,
	}})
	if err == nil || !strings.Contains(err.Error(), "invalid JSON arguments") {
		t.Fatalf("expected invalid-JSON error, got %v", err)
	}
}

func TestToolArgsSize(t *testing.T) {
	t.Parallel()
	if got := ToolArgsSize(nil); got != 0 {
		t.Errorf("ToolArgsSize(nil) = %d, want 0", got)
	}
	calls := []ToolCall{{InputDelta: "abc"}, {InputDelta: "de"}}
	if got := ToolArgsSize(calls); got != 5 {
		t.Errorf("ToolArgsSize() = %d, want 5", got)
	}
}

func FuzzMergeToolDelta(f *testing.F) {
	f.Add("a", "search", 0, `{"q":`)
	f.Add("", "calc", -1, `1}`)
	f.Add("", "", 2, `x`)
	f.Fuzz(func(t *testing.T, id, name string, index int, input string) {
		calls := MergeToolDelta(nil, ToolCall{ID: id, Name: name, Index: index, InputDelta: input})
		calls = MergeToolDelta(calls, ToolCall{ID: id, Name: name, Index: index, InputDelta: input})
		if len(calls) == 0 {
			t.Fatalf("merge produced no calls for id=%q name=%q index=%d", id, name, index)
		}
		if _, err := ToolUseBlocks(calls); err != nil {
			if json.Valid([]byte(calls[0].InputDelta)) {
				t.Fatalf("valid JSON %q rejected: %v", calls[0].InputDelta, err)
			}
		}
	})
}
