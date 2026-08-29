package testutil

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/kbukum/gokit/embedding"
	"github.com/kbukum/gokit/embedding/inmem"
)

// defaultDimensions is the vector width used when none is configured.
const defaultDimensions = 8

// FakeProvider is a deterministic, concurrency-safe [embedding.Provider] test double.
//
// Configure it with functional options. Each call resolves in order: if the block-until-cancel mode is set it waits for ctx cancellation; an injected error is returned next; then a dynamic responder if set; otherwise each input is embedded into a fixed-dimension vector — a pinned vector when one is registered for the text (see [WithVector]), else a deterministic hash of the text via an embedded [inmem.Provider]. Pinned vectors of differing lengths let a test exercise dimension-mismatch paths.
type FakeProvider struct {
	name       string
	dimensions int
	hash       *inmem.Provider

	err           error
	responder     func(context.Context, embedding.EmbedRequest) (embedding.EmbedResponse, error)
	blockOnCancel bool
	pinned        map[string][]float32

	mu    sync.Mutex
	calls int
}

// FakeOption configures a [FakeProvider].
type FakeOption func(*FakeProvider)

// WithName sets the provider name reported by Name.
func WithName(name string) FakeOption {
	return func(p *FakeProvider) { p.name = name }
}

// WithDimensions sets the width of deterministically hashed vectors. Values <= 0 are ignored, keeping the default.
func WithDimensions(dimensions int) FakeOption {
	return func(p *FakeProvider) {
		if dimensions > 0 {
			p.dimensions = dimensions
		}
	}
}

// WithError makes every Execute/EmbedBatch call fail with err.
func WithError(err error) FakeOption {
	return func(p *FakeProvider) { p.err = err }
}

// WithResponder installs a dynamic response function, evaluated per Execute call. It takes precedence over the default hashing but not over an injected error or block-until-cancel mode.
func WithResponder(fn func(context.Context, embedding.EmbedRequest) (embedding.EmbedResponse, error)) FakeOption {
	return func(p *FakeProvider) { p.responder = fn }
}

// WithVector pins text to vec, so every embedding of text returns vec verbatim. Pinning texts to vectors of differing lengths exercises dimension-mismatch handling in consumers. The slice is cloned on registration, so a later mutation of the caller's slice cannot change subsequent responses.
func WithVector(text string, vec []float32) FakeOption {
	return func(p *FakeProvider) {
		if p.pinned == nil {
			p.pinned = make(map[string][]float32)
		}
		p.pinned[text] = slices.Clone(vec)
	}
}

// WithBlockUntilCancel makes Execute block until ctx is canceled, then return ctx.Err(). Used to exercise timeout and cancellation paths deterministically without sleeps.
func WithBlockUntilCancel() FakeOption {
	return func(p *FakeProvider) { p.blockOnCancel = true }
}

// NewFakeProvider builds a FakeProvider with default dimensions, overridden by opts.
func NewFakeProvider(opts ...FakeOption) *FakeProvider {
	p := &FakeProvider{name: "embedding-fake", dimensions: defaultDimensions}
	for _, opt := range opts {
		opt(p)
	}
	p.hash = inmem.New(p.dimensions)
	return p
}

// Name reports the configured provider name.
func (p *FakeProvider) Name() string { return p.name }

// IsAvailable always reports true.
func (p *FakeProvider) IsAvailable(context.Context) bool { return true }

// Execute embeds every input in req, honoring cancellation and injected behavior.
func (p *FakeProvider) Execute(ctx context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
	// Count at entry so canceled and block-until-cancel calls are still recorded;
	// a timeout test must see that Execute was invoked.
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()

	if p.blockOnCancel {
		<-ctx.Done()
		return embedding.EmbedResponse{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return embedding.EmbedResponse{}, err
	}

	if p.err != nil {
		return embedding.EmbedResponse{}, p.err
	}
	if p.responder != nil {
		return p.responder(ctx, req)
	}

	embeddings := make([]embedding.Embedding, len(req.Inputs))
	for i, input := range req.Inputs {
		text, err := inputText(input)
		if err != nil {
			return embedding.EmbedResponse{}, err
		}
		vec, err := p.vector(ctx, text)
		if err != nil {
			return embedding.EmbedResponse{}, err
		}
		embeddings[i] = embedding.Embedding{Vector: vec, Dimensions: len(vec), Index: i}
	}
	resp := embedding.EmbedResponse{Model: req.Model}
	if len(embeddings) > 0 {
		resp.Embedding = embeddings[0]
		resp.Embeddings = embeddings
	}
	return resp, nil
}

// EmbedBatch embeds each request independently, honoring the same behavior as Execute.
func (p *FakeProvider) EmbedBatch(ctx context.Context, reqs []embedding.EmbedRequest) ([]embedding.EmbedResponse, error) {
	responses := make([]embedding.EmbedResponse, len(reqs))
	for i, req := range reqs {
		resp, err := p.Execute(ctx, req)
		if err != nil {
			return nil, err
		}
		responses[i] = resp
	}
	return responses, nil
}

// Calls reports how many times Execute has been invoked.
func (p *FakeProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// vector returns the pinned vector for text when registered, else a deterministic hash-derived vector of the configured width produced by the embedded [inmem.Provider] (reusing its hashing rather than duplicating it).
func (p *FakeProvider) vector(ctx context.Context, text string) ([]float32, error) {
	if v, ok := p.pinned[text]; ok {
		return slices.Clone(v), nil
	}
	resp, err := p.hash.Execute(ctx, embedding.EmbedRequest{Inputs: []embedding.EmbedInput{embedding.Text{Text: text}}})
	if err != nil {
		return nil, err
	}
	return resp.Embedding.Vector, nil
}

// inputText extracts text from a text embedding input. Non-text inputs (image, audio, video, or a nil text pointer) are rejected with an error, mirroring the real [inmem.Provider] so this reusable double cannot mask a consumer that sends an unsupported modality.
func inputText(input embedding.EmbedInput) (string, error) {
	switch v := input.(type) {
	case embedding.Text:
		return v.Text, nil
	case *embedding.Text:
		if v != nil {
			return v.Text, nil
		}
	}
	return "", fmt.Errorf("embedding/testutil: input has unsupported type %T", input)
}

var _ embedding.Provider = (*FakeProvider)(nil)
