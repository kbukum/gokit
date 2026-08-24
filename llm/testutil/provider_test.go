package testutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/testutil"
)

func TestFakeProviderQueuedResponses(t *testing.T) {
	t.Parallel()
	p := testutil.NewFakeProvider(
		testutil.WithName("q"),
		testutil.WithResponses(testutil.TextResponse("one"), testutil.TextResponse("two")),
	)
	if p.Name() != "q" || !p.IsAvailable(context.Background()) {
		t.Fatalf("name/availability = %q/%v", p.Name(), p.IsAvailable(context.Background()))
	}
	first, err := p.Execute(context.Background(), llm.CompletionRequest{})
	if err != nil || first.Text() != "one" {
		t.Fatalf("first = %q, %v", first.Text(), err)
	}
	second, err := p.Execute(context.Background(), llm.CompletionRequest{})
	if err != nil || second.Text() != "two" {
		t.Fatalf("second = %q, %v", second.Text(), err)
	}
	if _, err := p.Execute(context.Background(), llm.CompletionRequest{}); err == nil {
		t.Fatal("exhausted queue must return an error")
	}
	if p.Calls() != 3 {
		t.Fatalf("Calls() = %d, want 3", p.Calls())
	}
}

func TestFakeProviderInjectedError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("provider down")
	p := testutil.NewFakeProvider(testutil.WithError(sentinel))
	if _, err := p.Execute(context.Background(), llm.CompletionRequest{}); !errors.Is(err, sentinel) {
		t.Fatalf("Execute err = %v, want %v", err, sentinel)
	}
	if _, err := p.Stream(context.Background(), llm.CompletionRequest{}); !errors.Is(err, sentinel) {
		t.Fatalf("Stream err = %v, want %v", err, sentinel)
	}
}

func TestFakeProviderResponderReceivesRequest(t *testing.T) {
	t.Parallel()
	p := testutil.NewFakeProvider(testutil.WithResponder(
		func(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
			return testutil.TextResponse("saw:" + req.SystemPrompt), nil
		},
	))
	resp, err := p.Execute(context.Background(), llm.CompletionRequest{SystemPrompt: "hi"})
	if err != nil || resp.Text() != "saw:hi" {
		t.Fatalf("responder = %q, %v", resp.Text(), err)
	}
	if got := p.LastRequest().SystemPrompt; got != "hi" {
		t.Fatalf("LastRequest = %q", got)
	}
}

func TestFakeProviderStreamEmitsUsageThenComplete(t *testing.T) {
	t.Parallel()
	resp := testutil.TextResponse("hello")
	resp.Usage = llm.Usage{InputTokens: 3, OutputTokens: 2}
	p := testutil.NewFakeProvider(testutil.WithResponses(resp))
	ch, err := p.Stream(context.Background(), llm.CompletionRequest{})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	var events []llm.StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if _, ok := events[0].(llm.UsageDelta); !ok {
		t.Fatalf("event[0] = %T, want UsageDelta", events[0])
	}
	done, ok := events[1].(llm.MessageComplete)
	if !ok || done.Response.Text() != "hello" {
		t.Fatalf("event[1] = %#v", events[1])
	}
}

func TestFakeProviderBlockUntilCancel(t *testing.T) {
	t.Parallel()
	p := testutil.NewFakeProvider(testutil.WithBlockUntilCancel())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Execute(ctx, llm.CompletionRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked execute err = %v, want context.Canceled", err)
	}
}

func TestFakeProviderCapabilitiesAndTokens(t *testing.T) {
	t.Parallel()
	p := testutil.NewFakeProvider(
		testutil.WithCapabilities(llm.Capabilities{JSONMode: true}),
		testutil.WithTokenCounter(func([]chat.Message) int { return 42 }),
	)
	if !p.Capabilities().JSONMode {
		t.Fatal("capabilities override lost")
	}
	if got := p.CountTokens([]chat.Message{chat.User("x")}); got != 42 {
		t.Fatalf("CountTokens = %d, want 42", got)
	}
}

// FakeProvider must satisfy llm.Provider at compile time.
var _ llm.Provider = (*testutil.FakeProvider)(nil)
