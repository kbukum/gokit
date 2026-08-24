package testutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/llm"
)

// FakeProvider is a deterministic, concurrency-safe [llm.Provider] test double.
//
// Configure it with functional options. Response selection resolves in order:
// an injected error is returned first; then a dynamic responder if set; then the
// next queued response; otherwise an error signaling the queue is exhausted.
// Block-until-cancel mode overrides all of these and waits for ctx cancellation.
type FakeProvider struct {
	name         string
	caps         llm.Capabilities
	tokenCounter func([]chat.Message) int

	err           error
	responder     func(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error)
	blockOnCancel bool

	mu        sync.Mutex
	responses []llm.CompletionResponse
	queueIdx  int
	calls     int
	lastReq   llm.CompletionRequest
}

// FakeOption configures a [FakeProvider].
type FakeOption func(*FakeProvider)

// WithName sets the provider name reported by Name.
func WithName(name string) FakeOption {
	return func(p *FakeProvider) { p.name = name }
}

// WithResponses queues responses returned by successive Execute/Stream calls.
func WithResponses(responses ...llm.CompletionResponse) FakeOption {
	return func(p *FakeProvider) { p.responses = responses }
}

// WithResponder installs a dynamic response function, evaluated per call.
// It takes precedence over queued responses.
func WithResponder(fn func(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error)) FakeOption {
	return func(p *FakeProvider) { p.responder = fn }
}

// WithError makes every Execute/Stream call fail with err.
func WithError(err error) FakeOption {
	return func(p *FakeProvider) { p.err = err }
}

// WithCapabilities overrides the reported capabilities.
func WithCapabilities(caps llm.Capabilities) FakeOption {
	return func(p *FakeProvider) { p.caps = caps }
}

// WithTokenCounter overrides the CountTokens implementation.
func WithTokenCounter(fn func([]chat.Message) int) FakeOption {
	return func(p *FakeProvider) { p.tokenCounter = fn }
}

// WithBlockUntilCancel makes Execute block until ctx is canceled, then return
// ctx.Err(); Stream propagates the same cancellation. Used to exercise timeout
// and cancellation paths deterministically without sleeps.
func WithBlockUntilCancel() FakeOption {
	return func(p *FakeProvider) { p.blockOnCancel = true }
}

// NewFakeProvider builds a FakeProvider with sensible defaults (streaming and
// tool-use capable, approximate token counting), overridden by opts.
func NewFakeProvider(opts ...FakeOption) *FakeProvider {
	p := &FakeProvider{
		name: "fake",
		caps: llm.Capabilities{
			Streaming:       true,
			ToolUse:         true,
			MaxInputTokens:  100000,
			MaxOutputTokens: 4096,
		},
		tokenCounter: chat.CountTokensApprox,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name reports the configured provider name.
func (p *FakeProvider) Name() string { return p.name }

// IsAvailable always reports true.
func (p *FakeProvider) IsAvailable(context.Context) bool { return true }

// Execute returns the next configured response, honoring cancellation.
func (p *FakeProvider) Execute(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	if p.blockOnCancel {
		<-ctx.Done()
		return llm.CompletionResponse{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return llm.CompletionResponse{}, err
	}

	p.mu.Lock()
	p.lastReq = req
	p.calls++
	p.mu.Unlock()

	if p.err != nil {
		return llm.CompletionResponse{}, p.err
	}
	if p.responder != nil {
		return p.responder(ctx, req)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.queueIdx >= len(p.responses) {
		return llm.CompletionResponse{}, fmt.Errorf("testutil: fake provider has no more responses (call %d)", p.calls)
	}
	resp := p.responses[p.queueIdx]
	p.queueIdx++
	return resp, nil
}

// Stream returns the next response as a two-event stream (usage then the
// assembled completion). An Execute error surfaces as a Stream setup error.
func (p *FakeProvider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamEvent, error) {
	resp, err := p.Execute(ctx, req)
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.UsageDelta{InputTokens: resp.Usage.InputTokens, OutputTokens: resp.Usage.OutputTokens}
	ch <- llm.MessageComplete{Response: resp}
	close(ch)
	return ch, nil
}

// Capabilities reports the configured capabilities.
func (p *FakeProvider) Capabilities() llm.Capabilities { return p.caps }

// CountTokens counts tokens with the configured counter.
func (p *FakeProvider) CountTokens(messages []chat.Message) int {
	return p.tokenCounter(messages)
}

// Calls reports how many times Execute has been invoked.
func (p *FakeProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// LastRequest returns the most recent request passed to Execute.
func (p *FakeProvider) LastRequest() llm.CompletionRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastReq
}

// TextResponse builds a CompletionResponse carrying a single assistant text
// message and a stop finish reason — the common case for a fake completion.
func TextResponse(text string) llm.CompletionResponse {
	return llm.CompletionResponse{
		Message:    chat.Assistant(text),
		StopReason: chat.FinishReasonStop,
	}
}
