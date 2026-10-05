package llm

import (
	"testing"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm/internal/streamwire"
)

func TestStreamRepeatedMetadataAndPartialUsage(t *testing.T) {
	a := newStreamAssembler("requested", defaultStreamLimits())
	starts := 0
	var usages []UsageDelta
	send := func(e StreamEvent) error {
		switch e := e.(type) {
		case MessageStart:
			starts++
		case UsageDelta:
			usages = append(usages, e)
		}
		return nil
	}
	meta := &chat.MessageStart{ID: "id", Model: "actual"}
	input, output := 7, 0
	for _, chunk := range []streamChunk{
		{Metadata: meta, Usage: &streamwire.UsageUpdate{InputTokens: &input}},
		{Metadata: meta, Reasoning: "think"},
		{Metadata: meta, Usage: &streamwire.UsageUpdate{OutputTokens: &output}, Done: true},
	} {
		if err := a.add(chunk, send); err != nil {
			t.Fatal(err)
		}
	}
	result, err := a.complete()
	if err != nil {
		t.Fatal(err)
	}
	if starts != 1 {
		t.Fatalf("same response started %d times", starts)
	}
	if len(usages) != 2 || usages[1].InputTokens != 7 || usages[1].OutputTokens != 0 || !result.UsageReported || result.Text() != "" {
		t.Fatalf("observations: %+v result=%+v", usages, result)
	}
	missing := newStreamAssembler("m", defaultStreamLimits())
	if err := missing.add(streamChunk{Done: true}, send); err != nil {
		t.Fatal(err)
	}
	empty, err := missing.complete()
	if err != nil || empty.UsageReported {
		t.Fatalf("absent accounting became known zero: %+v %v", empty, err)
	}
}
