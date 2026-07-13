package ai_test

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/ai"
)

func TestTextContent(t *testing.T) {
	t.Parallel()
	parts := ai.TextContent("hello")
	if len(parts) != 1 {
		t.Fatalf("len(parts) = %d, want 1", len(parts))
	}
	text, ok := parts[0].(ai.Text)
	if !ok {
		t.Fatalf("part[0] type = %T, want ai.Text", parts[0])
	}
	if text.Text != "hello" {
		t.Errorf("text = %q, want hello", text.Text)
	}
}

func TestTextOf(t *testing.T) {
	t.Parallel()
	blocks := []ai.ContentPart{
		ai.Text{Text: "foo"},
		ai.Image{Source: "memory"},
		ai.Text{Text: "bar"},
		ai.ToolResultBlock{ID: "x", Content: "ignored"},
	}
	if got := ai.TextOf(blocks); got != "foobar" {
		t.Errorf("TextOf() = %q, want foobar", got)
	}
	if got := ai.TextOf(nil); got != "" {
		t.Errorf("TextOf(nil) = %q, want empty", got)
	}
}

func TestBlockType(t *testing.T) {
	t.Parallel()
	cases := []struct {
		block interface{ BlockType() string }
		want  string
	}{
		{ai.Text{}, "text"},
		{ai.Image{}, "image"},
		{ai.Audio{}, "audio"},
		{ai.Video{}, "video"},
		{ai.File{}, "file"},
		{ai.ToolUseBlock{}, "tool_use"},
		{ai.ToolResultBlock{}, "tool_result"},
	}
	for _, c := range cases {
		if got := c.block.BlockType(); got != c.want {
			t.Errorf("%T.BlockType() = %q, want %q", c.block, got, c.want)
		}
	}
}

func TestNormalizeToolInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"nil", "", "{}"},
		{"whitespace only", "  \t\n ", "{}"},
		{"json null", "null", "{}"},
		{"padded null", "  null  ", "{}"},
		{"object trimmed", `  {"a":1}  `, `{"a":1}`},
		{"array passthrough", "[1,2,3]", "[1,2,3]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ai.NormalizeToolInput(json.RawMessage(tt.in))
			if string(got) != tt.want {
				t.Errorf("NormalizeToolInput(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func FuzzNormalizeToolInput(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("null"))
	f.Add([]byte("  {}  "))
	f.Add([]byte(`{"a":1}`))
	f.Add([]byte("[1,2,3]"))
	f.Add([]byte("not json"))
	f.Fuzz(func(t *testing.T, in []byte) {
		out := ai.NormalizeToolInput(json.RawMessage(in))
		// Postcondition: output is never empty and never the JSON null literal.
		if len(out) == 0 {
			t.Fatalf("normalized output is empty for input %q", in)
		}
		if string(out) == "null" {
			t.Fatalf("normalized output is null for input %q", in)
		}
	})
}
