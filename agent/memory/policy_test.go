package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/agent/memory"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
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

func TestFail(t *testing.T) {
	if _, err := (memory.Fail{}).Compact(context.Background(), nil, 0); !errors.Is(err, memory.ErrContextExceeded) {
		t.Fatalf("fail strategy err=%v", err)
	}
}

func TestSummarize(t *testing.T) {
	msgs := []chat.Message{chat.System("sys"), chat.User("old"), chat.Assistant("recent")}
	p := &summaryProvider{text: "summary"}
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

// summaryProvider is a minimal llm.Provider returning a fixed completion.
type summaryProvider struct{ text string }

func (p *summaryProvider) Name() string                       { return "summary" }
func (p *summaryProvider) IsAvailable(_ context.Context) bool { return true }
func (p *summaryProvider) Execute(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	return llm.CompletionResponse{Message: chat.Assistant(p.text), StopReason: chat.FinishReasonStop}, nil
}

func (p *summaryProvider) Stream(_ context.Context, _ llm.CompletionRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent)
	close(ch)
	return ch, nil
}
func (p *summaryProvider) Capabilities() llm.Capabilities { return llm.Capabilities{} }
func (p *summaryProvider) CountTokens(msgs []chat.Message) int {
	return chat.CountTokensApprox(msgs)
}
