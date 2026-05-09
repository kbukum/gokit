package llm

import (
	"context"
	"strings"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/llm/internal/streamwire"
	"github.com/kbukum/gokit/observability"
)

const tracerName = "github.com/kbukum/gokit/llm"

// AdapterProvider wraps an *Adapter to implement the Provider interface.
// It applies provider defaults/capabilities and emits canonical StreamEvent
// values, including a final MessageComplete event.
//
// Per locked decision D12 (NATIVE COMPONENT), AdapterProvider implements
// component.Component (Start/Stop/Health) so bootstrap auto-wires it as
// infrastructure with no consumer code.
type AdapterProvider struct {
	adapter   *Adapter
	model     string
	caps      Capabilities
	Defaults  func(req *CompletionRequest)
	lifecycle ai.Lifecycle
}

func NewProvider(a *Adapter, model string) *AdapterProvider {
	return &AdapterProvider{adapter: a, model: model, caps: Capabilities{ToolUse: true, Streaming: true}}
}

func (p *AdapterProvider) WithCapabilities(caps Capabilities) *AdapterProvider {
	p.caps = caps
	return p
}

func (p *AdapterProvider) WithDefaults(fn func(req *CompletionRequest)) *AdapterProvider {
	p.Defaults = fn
	return p
}

// Execute is the canonical RequestResponse method. Per D7 NATIVE EMBED,
// llm.Provider embeds provider.Streamable[CompletionRequest, CompletionResponse, StreamEvent]
// so the single-response method on the LLM provider IS Execute (the streaming
// counterpart is Stream).
func (p *AdapterProvider) Execute(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	p.applyDefaults(&req)
	ctx, span := observability.StartNamedSpan(ctx, tracerName, "llm.complete",
		observability.WithSpanKind(observability.SpanKindClient),
		observability.WithSpanAttributes(
			observability.StringAttribute(semconv.GenAISystem, p.adapter.dialect.Name()),
			observability.StringAttribute(semconv.GenAIRequestModel, req.Model),
			observability.IntAttribute(semconv.GenAIRequestMaxTokens, req.MaxTokens),
		),
	)
	defer span.End()
	resp, err := p.adapter.Execute(ctx, req)
	if err != nil {
		span.RecordError(err)
		return CompletionResponse{}, err
	}
	span.SetAttributes(
		observability.IntAttribute(semconv.GenAIUsageInputTokens, resp.Usage.InputTokens),
		observability.IntAttribute(semconv.GenAIUsageOutputTokens, resp.Usage.OutputTokens),
		observability.StringAttribute(semconv.GenAIResponseFinishReason, string(resp.StopReason)),
	)
	p.lifecycle.Touch()
	return resp, nil
}

// Name delegates to the underlying adapter.
func (p *AdapterProvider) Name() string { return p.adapter.Name() }

// IsAvailable delegates to the underlying adapter.
func (p *AdapterProvider) IsAvailable(ctx context.Context) bool { return p.adapter.IsAvailable(ctx) }

// --- component.Component (D12) ---

// Start performs a cheap warm-up (records ready). It deliberately does not
// dial the upstream provider — readiness is verified via IsAvailable / Health
// at first request.
func (p *AdapterProvider) Start(_ context.Context) error {
	p.lifecycle.MarkReady()
	return nil
}

// Stop closes the underlying REST client.
func (p *AdapterProvider) Stop(ctx context.Context) error {
	p.lifecycle.MarkStopped()
	return p.adapter.Close(ctx)
}

// Health reports the component health: healthy once Start has been called
// and the upstream provider is reachable; degraded if not yet warmed up.
func (p *AdapterProvider) Health(ctx context.Context) component.Health {
	if !p.lifecycle.Ready() {
		return component.Health{Name: p.Name(), Status: component.StatusDegraded, Message: "not started"}
	}
	if !p.adapter.IsAvailable(ctx) {
		return component.Health{Name: p.Name(), Status: component.StatusUnhealthy, Message: "upstream unreachable"}
	}
	msg := "ready"
	if last := p.lifecycle.LastCall(); !last.IsZero() {
		msg = "last_call=" + last.UTC().Format("2006-01-02T15:04:05Z")
	}
	return component.Health{Name: p.Name(), Status: component.StatusHealthy, Message: msg}
}

func (p *AdapterProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamEvent, error) {
	p.applyDefaults(&req)
	ctx, span := observability.StartNamedSpan(ctx, tracerName, "llm.stream",
		observability.WithSpanKind(observability.SpanKindClient),
		observability.WithSpanAttributes(
			observability.StringAttribute(semconv.GenAISystem, p.adapter.dialect.Name()),
			observability.StringAttribute(semconv.GenAIRequestModel, req.Model),
		),
	)
	chunkCh, model, err := p.adapter.streamChunks(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.End()
		return nil, err
	}
	if model == "" {
		model = p.model
	}
	p.lifecycle.Touch()
	rawCh := streamEventsFromChunks(chunkCh, model)
	out := make(chan StreamEvent, cap(rawCh)+1)
	go func() {
		defer close(out)
		defer span.End()
		for evt := range rawCh {
			if errEvt, ok := evt.(StreamError); ok && errEvt.Err != nil {
				span.RecordError(errEvt.Err)
			}
			out <- evt
		}
	}()
	return out, nil
}

func (p *AdapterProvider) Capabilities() Capabilities { return p.caps }
func (p *AdapterProvider) CountTokens(messages []chat.Message) int {
	return chat.CountTokensApprox(messages)
}

func (p *AdapterProvider) applyDefaults(req *CompletionRequest) {
	if p.Defaults != nil {
		p.Defaults(req)
	}
}

func mergeStreamToolDelta(calls []streamToolCall, delta streamToolCall) []streamToolCall {
	return streamwire.MergeToolDelta(calls, delta)
}

func streamEventsFromChunks(chunkCh <-chan streamChunk, model string) <-chan StreamEvent {
	eventCh := make(chan StreamEvent, 16)
	go func() {
		defer close(eventCh)
		var contentBuf strings.Builder
		var streamCalls []streamToolCall
		for chunk := range chunkCh {
			if chunk.Err != nil {
				eventCh <- StreamError{Err: chunk.Err}
				return
			}
			if chunk.Content != "" {
				contentBuf.WriteString(chunk.Content)
				eventCh <- TextDelta{Text: chunk.Content}
			}
			for _, tc := range chunk.ToolCalls {
				streamCalls = mergeStreamToolDelta(streamCalls, tc)
				eventCh <- ToolUseDelta{Index: tc.Index, ID: tc.ID, Name: tc.Name, InputDelta: tc.InputDelta}
			}
			if chunk.Done {
				break
			}
		}
		msg := chat.AssistantMessage{}
		if text := contentBuf.String(); text != "" {
			msg.Content = ai.TextContent(text)
		}
		toolCalls, err := streamwire.ToolUseBlocks(streamCalls)
		if err != nil {
			eventCh <- StreamError{Err: err}
			return
		}
		msg.ToolCalls = toolCalls
		stopReason := chat.FinishReasonStop
		if len(msg.ToolCalls) > 0 {
			stopReason = chat.FinishReasonToolUse
		}
		eventCh <- MessageComplete{Response: CompletionResponse{Message: msg, Model: model, StopReason: stopReason}}
	}()
	return eventCh
}
