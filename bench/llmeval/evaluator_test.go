package llmeval

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/bench"
	"github.com/kbukum/gokit/llm"
	llmtest "github.com/kbukum/gokit/llm/testutil"
)

type eventProvider struct {
	*llmtest.FakeProvider
	events []llm.StreamEvent
}

func (p *eventProvider) Stream(context.Context, llm.CompletionRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, len(p.events))
	for _, e := range p.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

type recorder struct {
	starts int
	usage  bench.TokenUsage
}

func (o *recorder) ContentStarted()                { o.starts++ }
func (o *recorder) ReportUsage(u bench.TokenUsage) { o.usage = u }

func TestObservedProviderEvents(t *testing.T) {
	t.Parallel()
	terminal := llm.MessageComplete{Response: llm.CompletionResponse{Message: chat.Assistant("ok")}}
	for _, tc := range []struct {
		name    string
		events  []llm.StreamEvent
		counter llm.TokenCounter
		starts  int
		source  bench.UsageSource
		fail    bool
	}{
		{"content", []llm.StreamEvent{llm.TextDelta{Text: ""}, llm.TextDelta{Text: "ok"}, terminal}, nil, 1, bench.UsageUnavailable, false},
		{"reasoning-tools", []llm.StreamEvent{llm.ReasoningDelta{}, llm.ToolUseStart{}, llm.ToolUseDelta{}, llm.ToolUseStop{}, terminal}, nil, 0, bench.UsageUnavailable, false},
		{"reported-zero", []llm.StreamEvent{llm.UsageDelta{}, terminal}, llm.HeuristicTokenCounter{}, 0, bench.UsageReported, false},
		{"counted", []llm.StreamEvent{terminal}, llm.HeuristicTokenCounter{}, 0, bench.UsageCounted, false},
		{"missing-terminal", []llm.StreamEvent{llm.TextDelta{Text: "ok"}}, nil, 1, bench.UsageUnavailable, true},
		{"stream-error", []llm.StreamEvent{ai.Error{Err: errors.New("fixture")}}, nil, 0, bench.UsageUnavailable, true},
		{"overflow", []llm.StreamEvent{llm.TextDelta{Text: "too long"}}, nil, 1, bench.UsageUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			provider := &eventProvider{FakeProvider: llmtest.NewFakeProvider(), events: tc.events}
			e, err := New(provider, Config[string]{Stream: true, Counter: tc.counter, MaxOutputBytes: 4, Request: func([]byte) llm.CompletionRequest { return llm.CompletionRequest{} }, Prediction: func(r llm.CompletionResponse) (bench.Prediction[string], error) {
				return bench.Prediction[string]{Label: r.Text()}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			o := &recorder{usage: bench.TokenUsage{Source: bench.UsageUnavailable}}
			pred, err := e.(bench.ObservedEvaluator[string]).ExecuteObserved(t.Context(), []byte("input"), o)
			if (err != nil) != tc.fail {
				t.Fatalf("prediction=%v err=%v", pred, err)
			}
			if o.starts != tc.starts || o.usage.Source != tc.source {
				t.Fatalf("observation=%+v", o)
			}
			if !e.IsAvailable(t.Context()) || e.Name() != "fake" {
				t.Fatal("provider identity not forwarded")
			}
		})
	}
}

func TestUnaryUsageAndValidation(t *testing.T) {
	t.Parallel()
	mapper := func(r llm.CompletionResponse) (bench.Prediction[string], error) {
		return bench.Prediction[string]{Label: r.Text()}, nil
	}
	request := func([]byte) llm.CompletionRequest { return llm.CompletionRequest{} }
	for _, reported := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "zero"}[reported], func(t *testing.T) {
			t.Parallel()
			p := llmtest.NewFakeProvider(llmtest.WithResponder(func(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
				return llm.CompletionResponse{Message: chat.Assistant("ok"), UsageReported: reported}, nil
			}))
			e, err := New(p, Config[string]{Request: request, Prediction: mapper})
			if err != nil {
				t.Fatal(err)
			}
			o := &recorder{usage: bench.TokenUsage{Source: bench.UsageUnavailable}}
			if _, err := e.(bench.ObservedEvaluator[string]).ExecuteObserved(t.Context(), nil, o); err != nil {
				t.Fatal(err)
			}
			if o.starts != 0 || (o.usage.Source == bench.UsageReported) != reported {
				t.Fatalf("observation=%+v", o)
			}
			if _, err := e.Execute(t.Context(), nil); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, cfg := range []Config[string]{{}, {Request: request, Prediction: mapper, MaxOutputBytes: -1}} {
		if _, err := New(llmtest.NewFakeProvider(), cfg); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	if _, err := New[string](nil, Config[string]{Request: request, Prediction: mapper}); err == nil {
		t.Fatal("nil provider accepted")
	}
}
