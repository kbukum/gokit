package memory_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kbukum/gokit/agent/memory"
	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
	llmtestutil "github.com/kbukum/gokit/llm/testutil"
)

func TestRingBuffer(t *testing.T) {
	msgs := []chat.Message{chat.System("sys"), chat.User("1"), chat.User("2"), chat.User("3")}
	got, err := (memory.RingBuffer{KeepLast: 2}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("ring got %d", len(got))
	}
}

func TestTruncate(t *testing.T) {
	msgs := []chat.Message{chat.System("sys"), chat.User("1"), chat.User("2"), chat.User("3")}
	got, err := (memory.Truncate{KeepLast: 2}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("truncate got %d", len(got))
	}
}

func TestSlidingWindow(t *testing.T) {
	msgs := []chat.Message{chat.System("sys"), chat.User("1"), chat.User("2"), chat.User("3")}
	got, err := (memory.SlidingWindow{TokenCounter: func([]chat.Message) int { return 1 }}).Compact(context.Background(), msgs, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("empty sliding window")
	}
}

// TestSlidingWindowDropsUntilFitPreservingSystem proves the drop-until-fit loop
// removes the oldest non-system messages one at a time and stops as soon as the
// window fits, keeping the leading system message anchored throughout.
func TestSlidingWindowDropsUntilFitPreservingSystem(t *testing.T) {
	msgs := []chat.Message{
		chat.System("sys"),
		chat.User("1"), chat.User("2"), chat.User("3"), chat.User("4"),
	}
	// Each non-system message counts as 1 token; the system message is free.
	counter := func(ms []chat.Message) int {
		n := 0
		for _, m := range ms {
			if _, ok := m.(chat.SystemMessage); !ok {
				n++
			}
		}
		return n
	}
	got, err := (memory.SlidingWindow{TokenCounter: counter}).Compact(context.Background(), msgs, 2)
	if err != nil {
		t.Fatal(err)
	}
	// system + last two non-system messages == 3 messages, within budget 2.
	if len(got) != 3 {
		t.Fatalf("expected system + 2 messages, got %d: %#v", len(got), got)
	}
	if _, ok := got[0].(chat.SystemMessage); !ok {
		t.Fatalf("system message must stay first, got %T", got[0])
	}
	if u, ok := got[2].(chat.UserMessage); !ok || ai.TextOf(u.Content) != "4" {
		t.Fatalf("last message unexpected: %#v", got[2])
	}
}

// TestSlidingWindowKeepsLastWhenSingleMessageStillTooLarge proves the loop
// terminates and returns the anchored system plus the final message even when
// no window satisfies the budget, rather than looping unbounded.
func TestSlidingWindowKeepsLastWhenSingleMessageStillTooLarge(t *testing.T) {
	msgs := []chat.Message{chat.System("sys"), chat.User("1"), chat.User("2"), chat.User("3")}
	got, err := (memory.SlidingWindow{TokenCounter: func([]chat.Message) int { return 999 }}).Compact(context.Background(), msgs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected system + last message, got %d", len(got))
	}
	if _, ok := got[0].(chat.SystemMessage); !ok {
		t.Fatalf("system message must be preserved, got %T", got[0])
	}
}

// TestSlidingWindowNoSystemMessage covers the branch where there is no leading
// system message to anchor.
func TestSlidingWindowNoSystemMessage(t *testing.T) {
	msgs := []chat.Message{chat.User("1"), chat.User("2"), chat.User("3")}
	got, err := (memory.SlidingWindow{TokenCounter: func([]chat.Message) int { return 999 }}).Compact(context.Background(), msgs, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected single last message, got %d", len(got))
	}
}

func TestFail(t *testing.T) {
	if _, err := (memory.Fail{}).Compact(context.Background(), nil, 0); !errors.Is(err, memory.ErrContextExceeded) {
		t.Fatalf("fail strategy err=%v", err)
	}
}

func TestSummarize(t *testing.T) {
	msgs := []chat.Message{chat.System("sys"), chat.User("old"), chat.Assistant("recent")}
	p := llmtestutil.NewFakeProvider(llmtestutil.WithResponder(
		func(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
			return llmtestutil.TextResponse("summary"), nil
		},
	))
	got, err := (memory.Summarize{Provider: p, KeepLast: 1}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("summary got %d", len(got))
	}
	got, err = (memory.Summarize{KeepLast: 1}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("fallback got %d", len(got))
	}
}

// TestSummarizeIncludesAllRoleTranscripts proves the pre-summary transcript
// renders user, assistant, and tool-result turns before handing them to the
// provider, and that the returned summary is anchored after the system message.
func TestSummarizeIncludesAllRoleTranscripts(t *testing.T) {
	msgs := []chat.Message{
		chat.System("sys"),
		chat.User("question"),
		chat.Assistant("answer"),
		chat.ToolResultMsg("call_1", "tool output", false),
		chat.User("recent-1"),
		chat.User("recent-2"),
	}
	var captured string
	p := llmtestutil.NewFakeProvider(llmtestutil.WithResponder(
		func(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
			captured = transcriptOf(req)
			return llmtestutil.TextResponse("SUMMARY"), nil
		},
	))
	got, err := (memory.Summarize{Provider: p, KeepLast: 2}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[0].(chat.SystemMessage); !ok {
		t.Fatalf("system message must remain first, got %T", got[0])
	}
	sm, ok := got[1].(chat.SystemMessage)
	if !ok || sm.Content == "" {
		t.Fatalf("summary must be inserted as a system message, got %#v", got[1])
	}
	for _, want := range []string{"User: question", "Assistant: answer", "Tool(call_1): tool output"} {
		if !strings.Contains(captured, want) {
			t.Fatalf("transcript missing %q; got:\n%s", want, captured)
		}
	}
}

// TestSummarizeFallsBackWhenProviderFails proves a provider error keeps the
// recent messages verbatim (no summary injected) rather than failing the turn.
func TestSummarizeFallsBackWhenProviderFails(t *testing.T) {
	msgs := []chat.Message{
		chat.System("sys"),
		chat.User("old"),
		chat.User("recent-1"),
		chat.User("recent-2"),
	}
	p := llmtestutil.NewFakeProvider(llmtestutil.WithError(errors.New("provider unavailable")))
	got, err := (memory.Summarize{Provider: p, KeepLast: 2}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatalf("provider failure must not fail compaction: %v", err)
	}
	// system + 2 recent, no summary message inserted.
	if len(got) != 3 {
		t.Fatalf("expected system + 2 recent, got %d", len(got))
	}
	if _, ok := got[1].(chat.SystemMessage); ok {
		t.Fatal("no summary should be inserted when the provider fails")
	}
}

// transcriptOf extracts the rendered conversation transcript the Summarize
// policy sends to the provider (a single user message carrying the transcript).
func transcriptOf(req llm.CompletionRequest) string {
	var b strings.Builder
	for _, m := range req.Messages {
		if u, ok := m.(chat.UserMessage); ok {
			b.WriteString(ai.TextOf(u.Content))
		}
	}
	return b.String()
}

// TestRingBufferDefaultKeepAndNoSystem covers the default KeepLast (20) and the
// no-leading-system branch of RingBuffer.
func TestRingBufferDefaultKeepAndNoSystem(t *testing.T) {
	// KeepLast<=0 defaults to 20: a 5-message history is returned unchanged.
	short := []chat.Message{chat.User("1"), chat.User("2"), chat.User("3")}
	got, err := (memory.RingBuffer{}).Compact(context.Background(), short, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("default keep dropped messages: got %d", len(got))
	}

	// Without a leading system message, only the last KeepLast are kept.
	msgs := []chat.Message{chat.User("1"), chat.User("2"), chat.User("3")}
	got, err = (memory.RingBuffer{KeepLast: 1}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if _, ok := got[0].(chat.SystemMessage); ok {
		t.Fatal("no system message should be synthesized")
	}
}

// TestTruncateSmallHistoryUnchanged covers the below-threshold branch.
func TestTruncateSmallHistoryUnchanged(t *testing.T) {
	msgs := []chat.Message{chat.User("1"), chat.User("2")}
	got, err := (memory.Truncate{KeepLast: 5}).Compact(context.Background(), msgs, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected unchanged history, got %d", len(got))
	}
}
