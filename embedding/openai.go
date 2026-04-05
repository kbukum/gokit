package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// OpenAIConfig represents configuration for an OpenAI-compatible embedding provider.
type OpenAIConfig struct {
	// Endpoint is the base URL for the API (e.g., "https://api.openai.com").
	Endpoint string
	// APIKey is the API key for authentication. Empty string disables authentication.
	APIKey string
	// Model is the model name (e.g., "text-embedding-3-small").
	Model string
	// Dimensions is the expected embedding dimensions.
	Dimensions int
}

// DefaultOpenAIConfig returns the default configuration for OpenAI.
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		Endpoint:   "https://api.openai.com",
		APIKey:     "",
		Model:      "text-embedding-3-small",
		Dimensions: 1536,
	}
}

// OpenAIProvider implements Provider for OpenAI-compatible APIs.
// Works with OpenAI, Azure OpenAI, local llama.cpp, vLLM, or any server
// that exposes the /v1/embeddings endpoint.
type OpenAIProvider struct {
	client *http.Client
	config OpenAIConfig
}

// NewOpenAIProvider creates a new OpenAI embedding provider with the given configuration.
func NewOpenAIProvider(config OpenAIConfig) *OpenAIProvider {
	return &OpenAIProvider{
		client: &http.Client{},
		config: config,
	}
}

// NewOpenAIProviderWithClient creates a new OpenAI embedding provider with a custom HTTP client.
func NewOpenAIProviderWithClient(config OpenAIConfig, client *http.Client) *OpenAIProvider {
	return &OpenAIProvider{
		client: client,
		config: config,
	}
}

// Embed generates an embedding for a single text input.
func (p *OpenAIProvider) Embed(ctx context.Context, text string) ([]float32, error) {
	results, err := p.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	return results[0], nil
}

// EmbedBatch generates embeddings for a batch of texts.
func (p *OpenAIProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	endpoint := p.config.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com"
	}

	url := fmt.Sprintf("%s/v1/embeddings", endpoint)

	reqBody := map[string]interface{}{
		"model": p.config.Model,
		"input": texts,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.config.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w", err)
	}

	// Sort by index to match input order
	embeddings := make([][]float32, len(texts))
	for _, item := range result.Data {
		if item.Index < len(embeddings) {
			embeddings[item.Index] = item.Embedding
		}
	}

	return embeddings, nil
}

// Dimensions returns the configured embedding dimensions.
func (p *OpenAIProvider) Dimensions() int {
	return p.config.Dimensions
}
