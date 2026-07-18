// Package tgi provides an OpenAI-compatible inference adapter for Hugging Face Text Generation Inference (https://github.com/huggingface/text-generation-inference).
// TGI exposes an OpenAI-compatible /v1/completions endpoint at http://localhost:3000 by default.
// Per locked decision D4,
// this is a thin adapter that pass-throughs to the shared OAI-compat helper in package inference.
package tgi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/httpclient/sse"
	"github.com/kbukum/gokit/inference"
)

// Kind is the registry key for the TGI adapter.
const Kind = "tgi"

// DefaultBaseURL is TGI's default OpenAI-compatible listen address.
const DefaultBaseURL = "http://localhost:3000"

// Config configures the TGI adapter.
type Config struct {
	BaseURL     string `json:"base_url"`
	BearerToken string `json:"bearer_token,omitempty"`
}

// Provider is the live TGI adapter wrapping TGI's /v1/completions.
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
		return nil, fmt.Errorf("tgi: create http client: %w", err)
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

// Register adds the TGI factory to reg.
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

// Descriptor advertises the live TGI adapter.
func (p *Provider) Descriptor() inference.Descriptor {
	return inference.Descriptor{
		Name:            Kind,
		Description:     "Hugging Face TGI OpenAI-compatible text-generation inference adapter",
		ServingProtocol: "openai-v1-completions",
		Capabilities:    inference.CapabilityHints{SupportsStreaming: true},
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

// PredictStream streams /v1/completions via the shared OAI-compat helper.
func (p *Provider) PredictStream(ctx context.Context, req inference.PredictRequest) (<-chan ai.StreamEvent, error) {
	ch, err := inference.OAICompatPredictStream(ctx, Kind, p.openStream, req)
	if err == nil {
		p.lifecycle.Touch()
	}
	return ch, err
}

func (p *Provider) openStream(ctx context.Context, path string, body json.RawMessage) (sse.Reader, error) {
	resp, err := p.client.DoStream(ctx, httpclient.Request{Method: http.MethodPost, Path: path, Body: body})
	if err != nil {
		return nil, err
	}
	if resp.SSE == nil {
		_ = resp.Close()
		return nil, fmt.Errorf("tgi: server did not return an SSE stream")
	}
	return resp.SSE, nil
}

func (p *Provider) exec(ctx context.Context, method, path string, body any) ([]byte, error) {
	if method != http.MethodPost {
		return nil, fmt.Errorf("tgi: unsupported method %s", method)
	}
	resp, err := httpclient.Post[json.RawMessage](ctx, p.client, path, body)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

var (
	_ inference.Inference          = (*Provider)(nil)
	_ inference.StreamingInference = (*Provider)(nil)
	_ component.Component          = (*Provider)(nil)
)
