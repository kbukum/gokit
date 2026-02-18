package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/provider"
)

const (
	// ProviderName is the registered name for the Ollama provider.
	ProviderName = "ollama"

	defaultOllamaURL   = "http://localhost:11434"
	defaultOllamaModel = "llama3"
	defaultTimeout     = 120 * time.Second
)

// Config holds configuration for the Ollama provider.
type Config struct {
	BaseURL     string        `json:"base_url"`
	Model       string        `json:"model"`
	Temperature float64       `json:"temperature"`
	Timeout     time.Duration `json:"timeout"`
}

// Provider implements llm.Provider using Ollama's HTTP API.
type Provider struct {
	cfg    Config
	client *http.Client
}

// NewProvider creates a new Ollama LLM provider.
func NewProvider(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultOllamaURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultOllamaModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	return &Provider{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Factory returns a provider.Factory that creates Ollama Provider instances
// from a generic config map.
func Factory() provider.Factory[llm.Provider] {
	return func(cfg map[string]any) (llm.Provider, error) {
		oc := Config{}
		if v, ok := cfg["base_url"].(string); ok {
			oc.BaseURL = v
		}
		if v, ok := cfg["model"].(string); ok {
			oc.Model = v
		}
		if v, ok := cfg["temperature"].(float64); ok {
			oc.Temperature = v
		}
		if v, ok := cfg["timeout"].(time.Duration); ok {
			oc.Timeout = v
		}
		return NewProvider(oc), nil
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return ProviderName }

// IsAvailable checks if the Ollama server is reachable.
func (p *Provider) IsAvailable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+"/api/tags", http.NoBody)
	if err != nil {
		return false
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Complete sends a completion request and returns the full response.
func (p *Provider) Complete(ctx context.Context, req llm.CompletionRequest) (*llm.CompletionResponse, error) {
	chatReq := p.buildChatRequest(req, false, nil)

	resp, err := p.doRequest(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("ollama complete: %w", err)
	}

	return &llm.CompletionResponse{
		Content: resp.Message.Content,
		Model:   resp.Model,
		Usage: llm.Usage{
			PromptTokens:     resp.PromptEvalCount,
			CompletionTokens: resp.EvalCount,
			TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
		},
	}, nil
}

// CompleteStructured sends a completion request with JSON format mode.
func (p *Provider) CompleteStructured(ctx context.Context, req llm.CompletionRequest, schema any) (*llm.CompletionResponse, error) {
	var format any = "json"
	if schema != nil {
		format = schema
	}
	chatReq := p.buildChatRequest(req, false, format)

	resp, err := p.doRequest(ctx, chatReq)
	if err != nil {
		return nil, fmt.Errorf("ollama complete structured: %w", err)
	}

	return &llm.CompletionResponse{
		Content: resp.Message.Content,
		Model:   resp.Model,
		Usage: llm.Usage{
			PromptTokens:     resp.PromptEvalCount,
			CompletionTokens: resp.EvalCount,
			TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
		},
	}, nil
}

// Stream sends a completion request and returns a channel of streamed chunks.
func (p *Provider) Stream(ctx context.Context, req llm.CompletionRequest) (<-chan llm.StreamChunk, error) {
	chatReq := p.buildChatRequest(req, true, nil)

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("ollama stream: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama stream: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	//nolint:bodyclose // Body is closed in the goroutine that processes the stream
	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama stream: send request: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		_ = httpResp.Body.Close()
		return nil, fmt.Errorf("ollama stream: unexpected status %d: %s", httpResp.StatusCode, string(respBody))
	}

	ch := make(chan llm.StreamChunk)
	go func() {
		defer close(ch)
		defer httpResp.Body.Close() //nolint:errcheck // Error on close is safe to ignore for read operations

		scanner := bufio.NewScanner(httpResp.Body)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var resp ollamaChatResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				ch <- llm.StreamChunk{Err: fmt.Errorf("ollama stream: unmarshal chunk: %w", err)}
				return
			}

			chunk := llm.StreamChunk{
				Content: resp.Message.Content,
				Done:    resp.Done,
			}

			select {
			case ch <- chunk:
			case <-ctx.Done():
				ch <- llm.StreamChunk{Err: ctx.Err()}
				return
			}

			if resp.Done {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			ch <- llm.StreamChunk{Err: fmt.Errorf("ollama stream: read response: %w", err)}
		}
	}()

	return ch, nil
}

// --- internal Ollama API types ---

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model       string              `json:"model"`
	Messages    []ollamaChatMessage `json:"messages"`
	Stream      bool                `json:"stream"`
	Format      any                 `json:"format,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
}

type ollamaChatResponse struct {
	Model           string            `json:"model"`
	Message         ollamaChatMessage `json:"message"`
	Done            bool              `json:"done"`
	PromptEvalCount int               `json:"prompt_eval_count,omitempty"`
	EvalCount       int               `json:"eval_count,omitempty"`
}

// buildChatRequest creates an Ollama API request from a llm.CompletionRequest.
func (p *Provider) buildChatRequest(req llm.CompletionRequest, stream bool, format any) ollamaChatRequest {
	model := p.cfg.Model
	if req.Model != "" {
		model = req.Model
	}

	temp := p.cfg.Temperature
	if req.Temperature != 0 {
		temp = req.Temperature
	}

	msgs := make([]ollamaChatMessage, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		msgs = append(msgs, ollamaChatMessage{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, ollamaChatMessage{Role: m.Role, Content: m.Content})
	}

	return ollamaChatRequest{
		Model:       model,
		Messages:    msgs,
		Stream:      stream,
		Format:      format,
		Temperature: temp,
	}
}

// doRequest marshals the request, sends it to the Ollama API, and decodes the response.
func (p *Provider) doRequest(ctx context.Context, req ollamaChatRequest) (*ollamaChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck // Error on close is safe to ignore for read operations

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp ollamaChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &resp, nil
}
