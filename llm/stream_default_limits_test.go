package llm

import (
	"errors"
	"strings"
	"testing"
)

func TestStreamAssemblyDefaultLimits(t *testing.T) {
	send := func(StreamEvent) error { return nil }
	t.Run("output-8MiB", func(t *testing.T) {
		a := newStreamAssembler("m", defaultStreamLimits())
		chunk := strings.Repeat("x", 32<<10)
		for range 256 {
			if err := a.add(streamChunk{Content: chunk}, send); err != nil {
				t.Fatal(err)
			}
		}
		if a.content.Len() != 8<<20 {
			t.Fatalf("retained=%d", a.content.Len())
		}
		if err := a.add(streamChunk{Content: "x"}, send); !errors.Is(err, ErrStreamLimit) {
			t.Fatalf("one byte over: %v", err)
		}
		if a.content.Len() != 8<<20 {
			t.Fatal("overflow grew assembly")
		}
	})
	t.Run("arguments-1MiB", func(t *testing.T) {
		a := newStreamAssembler("m", defaultStreamLimits())
		args := `"` + strings.Repeat("x", (1<<20)-2) + `"`
		if err := a.add(streamChunk{ToolCalls: []streamToolCall{{Index: 0, ID: "id", Name: "tool", InputDelta: args}}, Done: true}, send); err != nil {
			t.Fatal(err)
		}
		result, err := a.complete()
		if err != nil || len(result.Message.ToolCalls) != 1 || len(result.Message.ToolCalls[0].Input) != 1<<20 {
			t.Fatalf("boundary: %v", err)
		}
		if err := a.add(streamChunk{ToolCalls: []streamToolCall{{Index: 0, InputDelta: " "}}}, send); !errors.Is(err, ErrStreamLimit) {
			t.Fatalf("one byte over: %v", err)
		}
	})
	t.Run("tools-128", func(t *testing.T) {
		a := newStreamAssembler("m", defaultStreamLimits())
		for i := range 128 {
			if err := a.add(streamChunk{ToolCalls: []streamToolCall{{Index: i, Name: "tool"}}}, send); err != nil {
				t.Fatal(err)
			}
		}
		if err := a.add(streamChunk{ToolCalls: []streamToolCall{{Index: 128, Name: "tool"}}}, send); !errors.Is(err, ErrStreamLimit) {
			t.Fatalf("one tool over: %v", err)
		}
		if len(a.calls) != 128 {
			t.Fatalf("retained=%d", len(a.calls))
		}
	})
}
