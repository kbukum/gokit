package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// EmbeddingProvider implements embedding.Provider for OpenAI-compatible APIs.
// Works with OpenAI, Azure OpenAI, vLLM, llama.cpp, or any server that
// exposes the /v1/embeddings endpoint.
type EmbeddingProvider struct {
	client *http.Client
	config Config
}

// NewEmbeddingProvider creates an OpenAI embedding provider.
func NewEmbeddingProvider(config Config) *EmbeddingProvider {
	config.applyDefaults()
	return &EmbeddingProvider{
		client: &http.Client{},
		config: config,
	}
}

// NewEmbeddingProviderWithClient creates an OpenAI embedding provider with a custom HTTP client.
func NewEmbeddingProviderWithClient(config Config, client *http.Client) *EmbeddingProvider {
	config.applyDefaults()
	return &EmbeddingProvider{
		client: client,
		config: config,
	}
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

	url := fmt.Sprintf("%s/embeddings", p.config.BaseURL)

	reqBody := map[string]any{
		"model": p.config.EmbeddingModel,
		"input": texts,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("openai: create embedding request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: embedding API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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
