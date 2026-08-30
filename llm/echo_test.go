package llm_test

import (
	"context"
	"testing"
	"time"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

func TestEchoName(t *testing.T) {
	t.Parallel()
	if got := (llm.Echo{}).Name(); got != llm.EchoName {
		t.Fatalf("Name() = %q, want %q", got, llm.EchoName)
	}
	if llm.EchoName != "echo" {
		t.Fatalf("EchoName = %q, want %q", llm.EchoName, "echo")
	}
}

func TestEchoSatisfiesProvider(t *testing.T) {
	t.Parallel()
	var _ llm.Provider = llm.Echo{}
}

func TestEchoIsAvailable(t *testing.T) {
	t.Parallel()
	if !(llm.Echo{}).IsAvailable(context.Background()) {
		t.Fatal("IsAvailable() = false, want true")
	}
}

func TestEchoCapabilities(t *testing.T) {
	t.Parallel()
	caps := (llm.Echo{}).Capabilities()
	if !caps.Streaming {
		t.Error("Capabilities().Streaming = false, want true")
	}
	if caps.ToolUse {
		t.Error("Capabilities().ToolUse = true, want false")
	}
}

func TestEchoCountTokensUsesSharedApproximation(t *testing.T) {
	t.Parallel()
	messages := []chat.Message{chat.User("hello world")}
	if got, want := (llm.Echo{}).CountTokens(messages), chat.CountTokensApprox(messages); got != want {
		t.Fatalf("CountTokens() = %d, want %d", got, want)
	}
}

func TestEchoExecuteEchoesLatestUserMessage(t *testing.T) {
	t.Parallel()
	req := llm.CompletionRequest{
		Messages: []chat.Message{
			chat.User("first"),
			chat.Assistant("ignored"),
			chat.User("most recent"),
		},
	}

	resp, err := (llm.Echo{}).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := resp.Text(); got != "most recent" {
		t.Fatalf("Text() = %q, want %q", got, "most recent")
	}
	if resp.Model != llm.EchoName {
		t.Fatalf("Model = %q, want %q", resp.Model, llm.EchoName)
	}
	if resp.StopReason != chat.FinishReasonStop {
		t.Fatalf("StopReason = %q, want %q", resp.StopReason, chat.FinishReasonStop)
	}
	if resp.Usage.InputTokens <= 0 {
		t.Errorf("Usage.InputTokens = %d, want > 0", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens <= 0 {
		t.Errorf("Usage.OutputTokens = %d, want > 0", resp.Usage.OutputTokens)
	}
}

func TestEchoUsageIsDeterministic(t *testing.T) {
	t.Parallel()
	req := llm.CompletionRequest{Messages: []chat.Message{chat.User("stable input")}}

	first, err := (llm.Echo{}).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	second, err := (llm.Echo{}).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if first.Usage != second.Usage {
		t.Fatalf("Usage not deterministic: %+v vs %+v", first.Usage, second.Usage)
	}

	wantInput := chat.CountTokensApprox(req.Messages)
	if first.Usage.InputTokens != wantInput {
		t.Errorf("Usage.InputTokens = %d, want %d", first.Usage.InputTokens, wantInput)
	}
	wantOutput := chat.CountTokensApprox([]chat.Message{chat.User(first.Text())})
	if first.Usage.OutputTokens != wantOutput {
		t.Errorf("Usage.OutputTokens = %d, want %d", first.Usage.OutputTokens, wantOutput)
	}
}

func TestEchoExecuteEmptyWhenNoUserMessage(t *testing.T) {
	t.Parallel()
	req := llm.CompletionRequest{Messages: []chat.Message{chat.System("only system")}}

	resp, err := (llm.Echo{}).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := resp.Text(); got != "" {
		t.Fatalf("Text() = %q, want empty", got)
	}
	if resp.Usage.OutputTokens != 0 {
		t.Fatalf("Usage.OutputTokens = %d, want 0", resp.Usage.OutputTokens)
	}
}

func TestEchoPreservesRequestedModel(t *testing.T) {
	t.Parallel()
	req := llm.CompletionRequest{
		Model:    "gpt-echo",
		Messages: []chat.Message{chat.User("hi")},
	}

	resp, err := (llm.Echo{}).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if resp.Model != "gpt-echo" {
		t.Fatalf("Model = %q, want %q", resp.Model, "gpt-echo")
	}
}

func TestEchoStreamEmitsReplyThenTerminal(t *testing.T) {
	t.Parallel()
	req := llm.CompletionRequest{Messages: []chat.Message{chat.User("stream me")}}

	ch, err := (llm.Echo{}).Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	var (
		text     string
		sawStart bool
		sawUsage bool
		terminal *llm.MessageComplete
	)
	for ev := range ch {
		switch e := ev.(type) {
		case llm.MessageStart:
			sawStart = true
		case llm.TextDelta:
			text += e.Text
		case llm.UsageDelta:
			sawUsage = true
		case llm.MessageComplete:
			terminal = &e
		}
	}

	if !sawStart {
		t.Error("stream did not emit MessageStart")
	}
	if !sawUsage {
		t.Error("stream did not emit UsageDelta")
	}
	if text != "stream me" {
		t.Errorf("streamed text = %q, want %q", text, "stream me")
	}
	if terminal == nil {
		t.Fatal("stream did not emit terminal MessageComplete")
	}
	if terminal.Response.Text() != "stream me" {
		t.Errorf("terminal response text = %q, want %q", terminal.Response.Text(), "stream me")
	}
}

func TestEchoInputUsageIncludesSystemPrompt(t *testing.T) {
	t.Parallel()
	messages := []chat.Message{chat.User("hi")}
	req := llm.CompletionRequest{
		SystemPrompt: "you are a helpful assistant",
		Messages:     messages,
	}

	resp, err := (llm.Echo{}).Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := chat.CountTokensApprox(append([]chat.Message{chat.System(req.SystemPrompt)}, messages...))
	if resp.Usage.InputTokens != want {
		t.Fatalf("Usage.InputTokens = %d, want %d (system prompt must be counted)", resp.Usage.InputTokens, want)
	}
	if bare := chat.CountTokensApprox(messages); resp.Usage.InputTokens <= bare {
		t.Fatalf("Usage.InputTokens = %d, want > messages-only count %d", resp.Usage.InputTokens, bare)
	}
}

func TestEchoStreamOmitsTextDeltaWhenReplyEmpty(t *testing.T) {
	t.Parallel()
	req := llm.CompletionRequest{Messages: []chat.Message{chat.System("only system")}}

	ch, err := (llm.Echo{}).Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	for ev := range ch {
		if _, ok := ev.(llm.TextDelta); ok {
			t.Fatal("stream emitted TextDelta for an empty reply")
		}
	}
}

// TestEchoStreamRejectsPreCanceledContext covers the early ctx.Err() guard: a
// context already canceled before Stream is called yields an error and no channel.
func TestEchoStreamRejectsPreCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := llm.CompletionRequest{Messages: []chat.Message{chat.User("canceled")}}
	ch, err := (llm.Echo{}).Stream(ctx, req)
	if err == nil {
		t.Fatal("Stream() with a pre-canceled context returned no error")
	}
	if ch != nil {
		t.Fatal("Stream() returned both error and channel")
	}
}

// TestEchoStreamUnwindsOnContextCancel exercises the guarded-send ctx.Done()
// branch: the stream starts, the consumer reads one event then abandons it by
// canceling. The producer goroutine must unwind and close the (unbuffered)
// channel promptly rather than blocking forever on the next send.
func TestEchoStreamUnwindsOnContextCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	req := llm.CompletionRequest{Messages: []chat.Message{chat.User("stream me")}}
	ch, err := (llm.Echo{}).Stream(ctx, req)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	if _, ok := <-ch; !ok {
		t.Fatal("stream closed before delivering any event")
	}
	cancel()

	done := make(chan struct{})
	go func() {
		for range ch { //nolint:revive // draining until closed after cancellation.
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not unwind after context cancel (goroutine leak)")
	}
}
