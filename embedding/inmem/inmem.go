// Package inmem provides a deterministic in-memory embedding adapter for tests.
package inmem

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/embedding"
	"github.com/kbukum/gokit/observability"
)

const defaultDimensions = 8

// Provider deterministically hashes text inputs into fixed-size vectors.
//
// Per locked decision D12 (NATIVE COMPONENT), Provider implements
// component.Component (Start/Stop/Health) so bootstrap auto-wires it.
type Provider struct {
	dimensions int
	lifecycle  ai.Lifecycle
}

// New creates a deterministic in-memory embedding provider.
func New(dimensions int) *Provider {
	if dimensions <= 0 {
		dimensions = defaultDimensions
	}
	return &Provider{dimensions: dimensions}
}

// Name returns the provider name.
func (p *Provider) Name() string { return "embedding-inmem" }

// IsAvailable always returns true for the in-memory provider.
func (p *Provider) IsAvailable(_ context.Context) bool { return true }

// Execute embeds all inputs in one request (provider.RequestResponse method).
func (p *Provider) Execute(ctx context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
	ctx, span := observability.StartNamedSpan(ctx, "github.com/kbukum/gokit/embedding/inmem", "embedding.embed",
		observability.WithSpanKind(observability.SpanKindInternal),
		observability.WithSpanAttributes(
			observability.StringAttribute(semconv.GenAISystem, "inmem"),
			observability.StringAttribute(semconv.GenAIOperationName, semconv.OpEmbedding),
			observability.StringAttribute(semconv.GenAIRequestModel, req.Model.Name),
			observability.IntAttribute("embedding.input_count", len(req.Inputs)),
		),
	)
	defer span.End()
	_ = ctx
	if len(req.Inputs) == 0 {
		return embedding.EmbedResponse{Model: model(req.Model), Usage: ai.Usage{}}, nil
	}
	embeddings := make([]embedding.Embedding, 0, len(req.Inputs))
	for i, input := range req.Inputs {
		var text string
		switch v := input.(type) {
		case embedding.Text:
			text = v.Text
		case *embedding.Text:
			if v == nil {
				err := fmt.Errorf("embedding/inmem: input %d has unsupported type %T", i, input)
				span.RecordError(err)
				return embedding.EmbedResponse{}, err
			}
			text = v.Text
		default:
			err := fmt.Errorf("embedding/inmem: input %d has unsupported type %T", i, input)
			span.RecordError(err)
			return embedding.EmbedResponse{}, err
		}
		embeddings = append(embeddings, embedding.Embedding{Vector: p.vector(text), Dimensions: p.dimensions, Index: i})
	}
	p.lifecycle.Touch()
	return embedding.EmbedResponse{Embedding: embeddings[0], Embeddings: embeddings, Model: model(req.Model), Usage: ai.Usage{}}, nil
}

// --- component.Component (D12) ---

// Start marks the provider ready (no upstream to warm up).
func (p *Provider) Start(_ context.Context) error { p.lifecycle.MarkReady(); return nil }

// Stop is a no-op for the in-memory provider.
func (p *Provider) Stop(_ context.Context) error { p.lifecycle.MarkStopped(); return nil }

// Health is always healthy once started.
func (p *Provider) Health(_ context.Context) component.Health {
	if !p.lifecycle.Ready() {
		return component.Health{Name: p.Name(), Status: component.StatusDegraded, Message: "not started"}
	}
	msg := "ready"
	if last := p.lifecycle.LastCall(); !last.IsZero() {
		msg = "last_call=" + last.UTC().Format("2006-01-02T15:04:05Z")
	}
	return component.Health{Name: p.Name(), Status: component.StatusHealthy, Message: msg}
}

// EmbedBatch embeds each request independently.
func (p *Provider) EmbedBatch(ctx context.Context, reqs []embedding.EmbedRequest) ([]embedding.EmbedResponse, error) {
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

func (p *Provider) vector(text string) []float32 {
	vec := make([]float32, p.dimensions)
	seed := []byte(text)
	for i := range vec {
		h := sha256.Sum256(append(seed, byte(i)))
		raw := binary.BigEndian.Uint32(h[:4])
		vec[i] = (float32(raw)/float32(math.MaxUint32))*2 - 1
	}
	return vec
}

func model(m ai.Model) ai.Model {
	if m.Name == "" {
		m.Name = "inmem-embedding"
	}
	if m.Provider == "" {
		m.Provider = ai.ProviderCustom
	}
	return m
}

var _ embedding.Provider = (*Provider)(nil)
