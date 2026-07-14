package convert

import (
	"encoding/json"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/tool"
)

func TestToMCPTool(t *testing.T) {
	t.Parallel()
	idempotent := true
	def := tool.Definition{
		Name:         "add",
		Description:  "Add numbers",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "object"},
		Annotations:  tool.Annotations{Title: "Adder", IdempotentHint: &idempotent},
	}
	def.Envelope.Safety = tool.SafetyReadOnly

	got := ToMCPTool(def)
	if got.Name != "add" || got.Description != "Add numbers" || got.Title != "Adder" {
		t.Fatalf("basic fields wrong: %+v", got)
	}
	if got.Annotations == nil || !got.Annotations.ReadOnlyHint || !got.Annotations.IdempotentHint {
		t.Fatalf("expected read-only + idempotent hints: %+v", got.Annotations)
	}
}

func TestToMCPToolOmitsEmptyAnnotations(t *testing.T) {
	t.Parallel()
	// A definition with unknown safety and no hints must not synthesize
	// annotations (avoids round-trip mutation of unset safety).
	got := ToMCPTool(tool.Definition{Name: "plain"})
	if got.Annotations != nil {
		t.Fatalf("expected nil annotations, got %+v", got.Annotations)
	}
}

func TestToDefinitionSafety(t *testing.T) {
	t.Parallel()
	destructive := true
	notDestructive := false
	cases := []struct {
		name string
		ann  *sdkmcp.ToolAnnotations
		want tool.Safety
	}{
		{"destructive", &sdkmcp.ToolAnnotations{DestructiveHint: &destructive}, tool.SafetyDestructive},
		{"read-only", &sdkmcp.ToolAnnotations{ReadOnlyHint: true}, tool.SafetyReadOnly},
		{"mutating", &sdkmcp.ToolAnnotations{DestructiveHint: &notDestructive}, tool.SafetyMutating},
		{"unknown", nil, ""},
	}
	for _, c := range cases {
		def := ToDefinition(&sdkmcp.Tool{Name: "t", Annotations: c.ann})
		if def.Envelope.Safety != c.want {
			t.Errorf("%s: got safety %q want %q", c.name, def.Envelope.Safety, c.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	safeties := []tool.Safety{tool.SafetyReadOnly, tool.SafetyMutating, tool.SafetyDestructive, ""}
	for _, s := range safeties {
		def := tool.Definition{Name: "tool", Description: "d", InputSchema: map[string]any{"type": "object"}}
		def.Envelope.Safety = s
		def.Annotations.Title = "Title"

		back := ToDefinition(ToMCPTool(def))
		if back.Envelope.Safety != s {
			t.Errorf("safety %q not preserved: got %q", s, back.Envelope.Safety)
		}
		if back.Annotations.Title != "Title" {
			t.Errorf("title not preserved for safety %q: %q", s, back.Annotations.Title)
		}
	}
}

func TestToSchemaJSON(t *testing.T) {
	t.Parallel()
	if m, ok := toSchemaJSON(map[string]any{"type": "object"}); !ok || m["type"] != "object" {
		t.Errorf("map passthrough failed: %v %v", m, ok)
	}
	if m, ok := toSchemaJSON(json.RawMessage(`{"type":"string"}`)); !ok || m["type"] != "string" {
		t.Errorf("raw message decode failed: %v %v", m, ok)
	}
	if m, ok := toSchemaJSON(json.RawMessage(`not json`)); ok {
		t.Errorf("invalid raw message must fail: %v", m)
	}
	type S struct {
		Type string `json:"type"`
	}
	if m, ok := toSchemaJSON(S{Type: "number"}); !ok || m["type"] != "number" {
		t.Errorf("struct marshal path failed: %v %v", m, ok)
	}
	if _, ok := toSchemaJSON(make(chan int)); ok {
		t.Error("unmarshalable value must fail")
	}
	if _, ok := toSchemaJSON(42); ok {
		t.Error("non-object JSON must fail to become schema map")
	}
}

func TestToMCPResult(t *testing.T) {
	t.Parallel()
	r := ToMCPResult(&tool.Result{Output: json.RawMessage(`{"sum":3}`), Content: "sum is 3"})
	if len(r.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(r.Content))
	}
	if r.StructuredContent == nil {
		t.Error("expected structured content from output")
	}
}

func TestToToolResult(t *testing.T) {
	t.Parallel()
	in := &sdkmcp.CallToolResult{
		Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: "line1"}, &sdkmcp.TextContent{Text: "line2"}},
		StructuredContent: map[string]any{"k": "v"},
	}
	got := ToToolResult(in)
	if got.Content != "line1\nline2" {
		t.Errorf("content join wrong: %q", got.Content)
	}
	if len(got.Output) == 0 {
		t.Error("expected output from structured content")
	}

	// Falls back to content-as-JSON when no structured content is present.
	got = ToToolResult(&sdkmcp.CallToolResult{Content: []sdkmcp.Content{&sdkmcp.TextContent{Text: `{"a":1}`}}})
	if string(got.Output) != `{"a":1}` {
		t.Errorf("expected JSON content promoted to output, got %q", got.Output)
	}
}

func TestToMCPAnnotationsOpenWorld(t *testing.T) {
	t.Parallel()
	def := tool.Definition{Name: "fetch", Description: "d"}
	def.Envelope.Filesystem = []tool.FilesystemRule{{Path: "/tmp", Mode: tool.FilesystemRead}}
	got := ToMCPTool(def)
	if got.Annotations == nil || got.Annotations.OpenWorldHint == nil || !*got.Annotations.OpenWorldHint {
		t.Fatalf("filesystem access must set open-world hint: %+v", got.Annotations)
	}
}

func FuzzToSchemaJSON(f *testing.F) {
	for _, s := range []string{`{"type":"object"}`, `not json`, `42`, `[1,2]`, `"str"`, ``} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		m, ok := toSchemaJSON(json.RawMessage(raw))
		if ok && m == nil {
			t.Fatalf("ok=true but nil map for %q", raw)
		}
	})
}

func FuzzMCPToolToDefinition(f *testing.F) {
	for _, s := range []string{
		`{"name":"a","description":"d"}`,
		`{"name":"b","inputSchema":{"type":"object"}}`,
		`{"name":"c","annotations":{"destructiveHint":true}}`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		var mt sdkmcp.Tool
		if err := json.Unmarshal([]byte(raw), &mt); err != nil {
			return
		}
		def := ToDefinition(&mt)
		// Name must always be carried across without loss.
		if def.Name != mt.Name {
			t.Fatalf("name lost: %q -> %q", mt.Name, def.Name)
		}
		// Safety must be one of the known closed vocabulary values.
		switch def.Envelope.Safety {
		case "", tool.SafetyReadOnly, tool.SafetyMutating, tool.SafetyDestructive:
		default:
			t.Fatalf("unexpected safety value %q", def.Envelope.Safety)
		}
	})
}
