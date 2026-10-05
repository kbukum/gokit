package llm

import (
	"errors"
	"io"
	"strings"
	"testing"

	"go.uber.org/goleak"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm/internal/streamwire"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func TestStreamAssemblyBoundsAndObservations(t *testing.T) {
	for _, over := range []bool{false, true} {
		t.Run(map[bool]string{false: "boundary", true: "overflow"}[over], func(t *testing.T) {
			a := newStreamAssembler("model", StreamLimits{MaxOutputBytes: 4, MaxToolArgsBytes: 4, MaxTools: 2})
			var events []StreamEvent
			send := func(e StreamEvent) error { events = append(events, e); return nil }
			n := 4
			if over {
				n++
			}
			err := a.add(streamChunk{Content: strings.Repeat("x", n)}, send)
			if over {
				if !errors.Is(err, ErrStreamLimit) || len(events) != 0 {
					t.Fatalf("overflow: events=%v err=%v", events, err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
	a := newStreamAssembler("requested", defaultStreamLimits())
	var events []StreamEvent
	send := func(e StreamEvent) error { events = append(events, e); return nil }
	zero := 0
	chunks := []streamChunk{
		{Metadata: &chat.MessageStart{ID: "id", Model: "actual"}},
		{Reasoning: "think"},
		{Content: "answer"},
		{Usage: &streamwire.UsageUpdate{InputTokens: &zero, OutputTokens: &zero}},
		{Done: true, StopReason: chat.FinishReasonLength},
	}
	for _, chunk := range chunks {
		if err := a.add(chunk, send); err != nil {
			t.Fatal(err)
		}
	}
	result, err := a.complete()
	if err != nil {
		t.Fatal(err)
	}
	if result.Text() != "answer" || result.Reasoning != "think" || !result.UsageReported || result.Model != "actual" || result.StopReason != chat.FinishReasonLength {
		t.Fatalf("lost observations: %+v", result)
	}
	if len(events) != 4 {
		t.Fatalf("events: %#v", events)
	}
	incomplete := newStreamAssembler("model", defaultStreamLimits())
	if _, err := incomplete.complete(); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("incomplete: %v", err)
	}
}

func TestStreamAssemblyToolLimits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		calls   []streamToolCall
		wantErr bool
	}{
		{"args-boundary", []streamToolCall{{Index: 0, InputDelta: "null"}}, false},
		{"args-over", []streamToolCall{{Index: 0, InputDelta: "null "}}, true},
		{"tools-boundary", []streamToolCall{{Index: 0}, {Index: 1}}, false},
		{"tools-over", []streamToolCall{{Index: 0}, {Index: 1}, {Index: 2}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newStreamAssembler("m", StreamLimits{MaxOutputBytes: 100, MaxToolArgsBytes: 4, MaxTools: 2})
			err := a.add(streamChunk{ToolCalls: tc.calls, Done: true}, func(StreamEvent) error { return nil })
			if errors.Is(err, ErrStreamLimit) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
		})
	}
}
