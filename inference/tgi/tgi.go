// Package tgi provides an OpenAI-compatible inference adapter for
// Hugging Face Text Generation Inference
// (https://github.com/huggingface/text-generation-inference). TGI exposes
// an OpenAI-compatible /v1/completions endpoint at http://localhost:3000
// by default. Per locked decision D4, this is a thin adapter that
// pass-throughs to the shared OAI-compat helper in package inference.
package tgi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/httpclient"
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
	cfg    Config
	client *httpclient.Adapter
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
	return inference.OAICompatPredict(ctx, Kind, p.exec, req)
}

// Descriptor advertises the live TGI adapter.
func (p *Provider) Descriptor() inference.Descriptor {
	return inference.Descriptor{
		Name:            Kind,
		Description:     "Hugging Face TGI OpenAI-compatible text-generation inference adapter",
		ServingProtocol: "openai-v1-completions",
		Capabilities:    inference.CapabilityHints{SupportsStreaming: false},
		Available:       true,
	}
}

func (p *Provider) exec(ctx context.Context, method, path string, body any) ([]byte, error) {
	if method != http.MethodPost {
		return nil, fmt.Errorf("tgi: unsupported method %s", method)
	}
	resp, err := httpclient.Post[json.RawMessage](p.client, ctx, path, body)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

var _ inference.Inference = (*Provider)(nil)
