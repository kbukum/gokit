package llm

import (
	"context"
	"strings"
)

// AdapterProvider wraps an *Adapter to implement the Provider interface.
// It bridges the adapter's StreamChunk-based streaming to the agent's
// StreamEvent protocol, which requires a final MessageComplete event.
type AdapterProvider struct {
	adapter *Adapter
	model   string
	caps    Capabilities
	// Defaults is called before each request to apply provider-specific defaults.
	// May be nil.
	Defaults func(req *CompletionRequest)
}

// NewProvider creates a Provider from an *Adapter.
func NewProvider(a *Adapter, model string) *AdapterProvider {
	return &AdapterProvider{
		adapter: a,
		model:   model,
		caps: Capabilities{
			SupportsTools:     true,
			SupportsStreaming: true,
			ModelID:           model,
		},
	}
}

// WithCapabilities sets custom capability flags.
func (p *AdapterProvider) WithCapabilities(caps Capabilities) *AdapterProvider {
	p.caps = caps
	if p.caps.ModelID == "" {
		p.caps.ModelID = p.model
	}
	return p
}

// WithDefaults sets a function called before each request to apply defaults.
func (p *AdapterProvider) WithDefaults(fn func(req *CompletionRequest)) *AdapterProvider {
	p.Defaults = fn
	return p
}

func (p *AdapterProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	p.applyDefaults(&req)
	resp, err := p.adapter.Execute(ctx, req)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (p *AdapterProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamEvent, error) {
	p.applyDefaults(&req)

	chunkCh, err := p.adapter.Stream(ctx, req)
	if err != nil {
		return nil, err
	}

	eventCh := make(chan StreamEvent, 16)
	go func() {
		defer close(eventCh)

		var contentBuf strings.Builder
		var toolCalls []ToolCall

		for chunk := range chunkCh {
			if chunk.Err != nil {
				eventCh <- StreamError{Err: chunk.Err}
				return
			}
			if chunk.Content != "" {
				contentBuf.WriteString(chunk.Content)
				eventCh <- ContentDelta{Text: chunk.Content}
			}
			for _, tc := range chunk.ToolCalls {
				toolCalls = mergeToolCallDelta(toolCalls, tc)
				eventCh <- ToolCallDelta{
					ID:         tc.ID,
					Name:       tc.Function.Name,
					InputDelta: tc.Function.Arguments,
				}
			}
			if chunk.Done {
				break
			}
		}

		msg := AssistantMessage{}
		if text := contentBuf.String(); text != "" {
			msg.Content = TextContent(text)
		}
		msg.ToolCalls = toolCalls

		stopReason := StopEndTurn
		if len(toolCalls) > 0 {
			stopReason = StopToolUse
		}

		eventCh <- MessageComplete{
			Response: CompletionResponse{
				Message:    msg,
				Model:      p.model,
				StopReason: stopReason,
			},
		}
	}()

	return eventCh, nil
}

func (p *AdapterProvider) Capabilities() Capabilities {
	return p.caps
}

func (p *AdapterProvider) CountTokens(messages []Message) int {
	return CountTokensApprox(messages)
}

func (p *AdapterProvider) applyDefaults(req *CompletionRequest) {
	if p.Defaults != nil {
		p.Defaults(req)
	}
}

// mergeToolCallDelta accumulates streaming tool call fragments into complete calls.
func mergeToolCallDelta(calls []ToolCall, delta ToolCall) []ToolCall {
	if delta.ID != "" {
		for i := range calls {
			if calls[i].ID == delta.ID {
				if delta.Function.Name != "" {
					calls[i].Function.Name = delta.Function.Name
				}
				calls[i].Function.Arguments += delta.Function.Arguments
				return calls
			}
		}
		return append(calls, delta)
	}
	if delta.Function.Name != "" {
		for i := range calls {
			if calls[i].Function.Name == delta.Function.Name {
				calls[i].Function.Arguments += delta.Function.Arguments
				return calls
			}
		}
		return append(calls, delta)
	}
	if len(calls) > 0 {
		calls[len(calls)-1].Function.Arguments += delta.Function.Arguments
		return calls
	}
	return append(calls, delta)
}
