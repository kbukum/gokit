package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/embedding"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/resilience"
)

// EmbeddingProvider implements embedding.Provider for OpenAI-compatible APIs.
//
// Per locked decision D12 (NATIVE COMPONENT), EmbeddingProvider implements
// component.Component so bootstrap auto-wires it as infrastructure.
type EmbeddingProvider struct {
	client    *httpclient.Adapter
	config    Config
	lifecycle ai.Lifecycle
	policy    *resilience.Policy
}

type EmbeddingProviderOption func(*EmbeddingProvider)

// WithPolicy wraps outbound embedding calls with an optional resilience policy.
func WithPolicy(policy *resilience.Policy) EmbeddingProviderOption {
	return func(p *EmbeddingProvider) {
		p.policy = policy
	}
}

// NewEmbeddingProvider creates an OpenAI embedding provider.
func NewEmbeddingProvider(config Config, opts ...EmbeddingProviderOption) *EmbeddingProvider {
	config.applyDefaults()

	httpCfg := httpclient.Config{Name: "openai-embedding", BaseURL: config.BaseURL}
	if config.APIKey != "" {
		httpCfg.Auth = httpclient.BearerAuth(config.APIKey)
	}
	client, err := httpclient.New(httpCfg)
	if err != nil {
		client, _ = httpclient.New(httpclient.Config{Name: "openai-embedding", BaseURL: config.BaseURL})
	}
	provider := &EmbeddingProvider{client: client, config: config}
	for _, opt := range opts {
		if opt != nil {
			opt(provider)
		}
	}
	return provider
}

// Name returns the provider name.
func (p *EmbeddingProvider) Name() string { return p.client.Name() }

// IsAvailable delegates to the underlying HTTP client.
func (p *EmbeddingProvider) IsAvailable(ctx context.Context) bool { return p.client.IsAvailable(ctx) }

// Execute generates embeddings for one canonical request (provider.RequestResponse method).
func (p *EmbeddingProvider) Execute(ctx context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
	ctx, span := observability.StartNamedSpan(ctx, "github.com/kbukum/gokit/llm/providers/openai", "embedding.embed",
		observability.WithSpanKind(observability.SpanKindClient),
		observability.WithSpanAttributes(
			observability.StringAttribute(semconv.GenAISystem, "openai"),
			observability.StringAttribute(semconv.GenAIOperationName, semconv.OpEmbedding),
			observability.StringAttribute(semconv.GenAIRequestModel, p.modelName(req.Model)),
			observability.IntAttribute("embedding.input_count", len(req.Inputs)),
		),
	)
	defer span.End()
	responses, err := p.EmbedBatch(ctx, []embedding.EmbedRequest{req})
	if err != nil {
		span.RecordError(err)
		return embedding.EmbedResponse{}, err
	}
	if len(responses) == 0 {
		err := fmt.Errorf("openai: empty embedding response")
		span.RecordError(err)
		return embedding.EmbedResponse{}, err
	}
	span.SetAttributes(
		observability.IntAttribute(semconv.GenAIUsageInputTokens, responses[0].Usage.InputTokens),
		observability.IntAttribute(semconv.GenAIUsageOutputTokens, responses[0].Usage.OutputTokens),
	)
	p.lifecycle.Touch()
	return responses[0], nil
}

// --- component.Component (D12) ---

// Start marks the provider as ready.
func (p *EmbeddingProvider) Start(_ context.Context) error { p.lifecycle.MarkReady(); return nil }

// Stop closes the underlying httpclient.
func (p *EmbeddingProvider) Stop(ctx context.Context) error {
	p.lifecycle.MarkStopped()
	return p.client.Close(ctx)
}

// Health reports component health based on upstream availability.
func (p *EmbeddingProvider) Health(ctx context.Context) component.Health {
	if !p.lifecycle.Ready() {
		return component.Health{Name: p.Name(), Status: component.StatusDegraded, Message: "not started"}
	}
	if !p.client.IsAvailable(ctx) {
		return component.Health{Name: p.Name(), Status: component.StatusUnhealthy, Message: "upstream unreachable"}
	}
	msg := "ready"
	if last := p.lifecycle.LastCall(); !last.IsZero() {
		msg = "last_call=" + last.UTC().Format("2006-01-02T15:04:05Z")
	}
	return component.Health{Name: p.Name(), Status: component.StatusHealthy, Message: msg}
}

// EmbedBatch generates embeddings for a batch of canonical requests.
func (p *EmbeddingProvider) EmbedBatch(ctx context.Context, reqs []embedding.EmbedRequest) ([]embedding.EmbedResponse, error) {
	responses := make([]embedding.EmbedResponse, len(reqs))
	for i, req := range reqs {
		resp, err := p.embedOne(ctx, req)
		if err != nil {
			return nil, err
		}
		responses[i] = resp
	}
	return responses, nil
}

func (p *EmbeddingProvider) embedOne(ctx context.Context, req embedding.EmbedRequest) (embedding.EmbedResponse, error) {
	if len(req.Inputs) == 0 {
		return embedding.EmbedResponse{Model: p.responseModel(req.Model), Usage: ai.Usage{}}, nil
	}
	texts := make([]string, len(req.Inputs))
	for i, input := range req.Inputs {
		text, ok := input.(embedding.Text)
		if !ok {
			return embedding.EmbedResponse{}, fmt.Errorf("openai: input %d has unsupported type %T", i, input)
		}
		texts[i] = text.Text
	}
	reqBody := map[string]any{"model": p.modelName(req.Model), "input": texts}
	for key, value := range req.Options {
		reqBody[key] = value
	}

	resp, err := resilience.Execute(ctx, p.policy, func(callCtx context.Context) (*httpclient.TypedResponse[json.RawMessage], error) {
		return httpclient.Post[json.RawMessage](p.client, callCtx, "/embeddings", reqBody)
	})
	if err != nil {
		return embedding.EmbedResponse{}, fmt.Errorf("openai: embedding request failed: %w", err)
	}
	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return embedding.EmbedResponse{}, fmt.Errorf("openai: parse embedding response: %w", err)
	}
	embeddings := make([]embedding.Embedding, len(texts))
	for _, item := range result.Data {
		if item.Index >= 0 && item.Index < len(embeddings) {
			embeddings[item.Index] = embedding.Embedding{Vector: item.Embedding, Dimensions: len(item.Embedding), Index: item.Index}
		}
	}
	usage := ai.Usage{InputTokens: result.Usage.PromptTokens}
	if result.Usage.TotalTokens > result.Usage.PromptTokens {
		usage.OutputTokens = result.Usage.TotalTokens - result.Usage.PromptTokens
	}
	return embedding.EmbedResponse{Embedding: embeddings[0], Embeddings: embeddings, Model: p.responseModel(req.Model), Usage: usage}, nil
}

func (p *EmbeddingProvider) modelName(model ai.Model) string {
	if model.Name != "" {
		return model.Name
	}
	return p.config.EmbeddingModel
}

func (p *EmbeddingProvider) responseModel(model ai.Model) ai.Model {
	if model.Name == "" {
		model.Name = p.config.EmbeddingModel
	}
	if model.Provider == "" {
		model.Provider = ai.ProviderOpenAI
	}
	return model
}

var _ embedding.Provider = (*EmbeddingProvider)(nil)
