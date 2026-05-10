// Package vllm provides an OpenAI-compatible inference adapter for vLLM
// (https://github.com/vllm-project/vllm), which exposes /v1/completions
// at http://localhost:8000 by default. Per locked decision D4, this is
// a thin adapter that pass-throughs to the shared OAI-compat helper in
// package inference.
package vllm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/inference"
)

// Kind is the registry key for the vLLM adapter.
const Kind = "vllm"

// DefaultBaseURL is vLLM's default OpenAI-compatible listen address.
const DefaultBaseURL = "http://localhost:8000"

// Config configures the vLLM adapter.
type Config struct {
	BaseURL     string `json:"base_url"`
	BearerToken string `json:"bearer_token,omitempty"`
}

// Provider is the live vLLM adapter wrapping vLLM's /v1/completions.
type Provider struct {
	cfg       Config
	client    *httpclient.Adapter
	lifecycle ai.Lifecycle
}

// New constructs a Provider from cfg.
func New(cfg Config) (*Provider, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	httpCfg := httpclient.Config{Name: Kind, BaseURL: cfg.BaseURL}
	if cfg.BearerToken != "" {
		httpCfg.Auth = &httpclient.AuthConfig{Type: httpclient.AuthBearer, Token: cfg.BearerToken}
	}
	client, err := httpclient.New(httpCfg)
	if err != nil {
		return nil, fmt.Errorf("vllm: create http client: %w", err)
	}
	return &Provider{cfg: cfg, client: client}, nil
}

// Factory builds the adapter from JSON config.
func Factory(config json.RawMessage) (inference.Inference, error) {
	var cfg Config
	if len(config) > 0 {
		if err := json.Unmarshal(config, &cfg); err != nil {
			return nil, err
		}
	}
	return New(cfg)
}

// Register adds the vLLM factory to reg.
func Register(reg *inference.Registry) error { return reg.Register(Kind, Factory) }

// Predict calls /v1/completions via the shared OAI-compat helper.
func (p *Provider) Predict(ctx context.Context, req inference.PredictRequest) (inference.PredictResponse, error) {
	resp, err := inference.OAICompatPredict(ctx, Kind, p.exec, req)
	if err == nil {
		p.lifecycle.Touch()
	}
	return resp, err
}

// Execute satisfies provider.RequestResponse by forwarding to Predict.
func (p *Provider) Execute(ctx context.Context, req inference.PredictRequest) (inference.PredictResponse, error) {
	return p.Predict(ctx, req)
}

// Descriptor advertises the live vLLM adapter.
func (p *Provider) Descriptor() inference.Descriptor {
	return inference.Descriptor{
		Name:            Kind,
		Description:     "vLLM OpenAI-compatible text-generation inference adapter",
		ServingProtocol: "openai-v1-completions",
		Capabilities:    inference.CapabilityHints{SupportsStreaming: false},
		Available:       true,
	}
}

// Name returns the adapter name.
func (p *Provider) Name() string { return Kind }

// IsAvailable reports whether the underlying HTTP client is reachable.
func (p *Provider) IsAvailable(ctx context.Context) bool { return p.client.IsAvailable(ctx) }

// Start marks the adapter ready after constructor validation.
func (p *Provider) Start(_ context.Context) error {
	p.lifecycle.MarkReady()
	return nil
}

// Stop closes idle HTTP connections.
func (p *Provider) Stop(ctx context.Context) error {
	p.lifecycle.MarkStopped()
	return p.client.Close(ctx)
}

// Health reports whether the adapter is ready to serve requests.
func (p *Provider) Health(ctx context.Context) component.Health {
	if !p.lifecycle.Ready() {
		return component.Health{Name: p.Name(), Status: component.StatusDegraded, Message: "not started"}
	}
	if !p.client.IsAvailable(ctx) {
		return component.Health{Name: p.Name(), Status: component.StatusUnhealthy, Message: "client unavailable"}
	}
	msg := "ready"
	if last := p.lifecycle.LastCall(); !last.IsZero() {
		msg = "last_call=" + last.UTC().Format("2006-01-02T15:04:05Z")
	}
	return component.Health{Name: p.Name(), Status: component.StatusHealthy, Message: msg}
}

func (p *Provider) exec(ctx context.Context, method, path string, body any) ([]byte, error) {
	if method != http.MethodPost {
		return nil, fmt.Errorf("vllm: unsupported method %s", method)
	}
	resp, err := httpclient.Post[json.RawMessage](p.client, ctx, path, body)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

var (
	_ inference.Inference = (*Provider)(nil)
	_ component.Component = (*Provider)(nil)
)
