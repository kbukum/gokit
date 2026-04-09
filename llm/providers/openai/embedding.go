package openai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/httpclient"
)

// EmbeddingProvider implements embedding.Provider for OpenAI-compatible APIs.
// Works with OpenAI, Azure OpenAI, vLLM, llama.cpp, or any server that
// exposes the /v1/embeddings endpoint.
//
// Uses gokit's httpclient for proper auth, TLS, timeouts, and resilience.
type EmbeddingProvider struct {
	client *httpclient.Adapter
	config Config
}

// NewEmbeddingProvider creates an OpenAI embedding provider.
func NewEmbeddingProvider(config Config) *EmbeddingProvider {
	config.applyDefaults()

	httpCfg := httpclient.Config{
		Name:    "openai-embedding",
		BaseURL: config.BaseURL,
	}
	if config.APIKey != "" {
		httpCfg.Auth = httpclient.BearerAuth(config.APIKey)
	}

	client, err := httpclient.New(httpCfg)
	if err != nil {
		client, _ = httpclient.New(httpclient.Config{Name: "openai-embedding", BaseURL: config.BaseURL})
	}

	return &EmbeddingProvider{client: client, config: config}
}

// Embed generates an embedding for a single text input.
func (p *EmbeddingProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("openai: empty embedding response")
	}
	return results[0], nil
}

// EmbedBatch generates embeddings for a batch of texts.
func (p *EmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	reqBody := map[string]any{
		"model": p.config.EmbeddingModel,
		"input": texts,
	}

	resp, err := httpclient.Post[json.RawMessage](p.client, ctx, "/embeddings", reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: embedding request failed: %w", err)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}

	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("openai: parse embedding response: %w", err)
	}

	embeddings := make([][]float32, len(texts))
	for _, item := range result.Data {
		if item.Index < len(embeddings) {
			embeddings[item.Index] = item.Embedding
		}
	}

	return embeddings, nil
}

// Dimensions returns the configured embedding dimensions.
func (p *EmbeddingProvider) Dimensions() int {
	return p.config.EmbeddingDimensions
}
