package triton

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/inference"
)

const servingProtocol = "kserve-v2-http"

// Provider implements inference.Inference against Triton / KServe v2 HTTP.
type Provider struct {
	cfg        Config
	client     *httpclient.Adapter
	descriptor inference.Descriptor
	decider    authz.Decider
	subject    authz.Subject
	lifecycle  ai.Lifecycle
}

// NewProvider creates a Triton KServe v2 HTTP provider.
func NewProvider(cfg Config, options ...Option) (*Provider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("triton: base_url is required")
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("triton: invalid base_url %q", cfg.BaseURL)
	}
	if cfg.Name == "" {
		cfg.Name = Kind
	}
	if cfg.Description == "" {
		cfg.Description = "Triton / KServe v2 HTTP model-serving adapter"
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}

	opts := providerOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	client := opts.httpClient
	if client == nil {
		httpCfg := httpclient.Config{
			Name:    cfg.Name,
			BaseURL: strings.TrimRight(cfg.BaseURL, "/"),
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
			Headers: cfg.Headers,
			Retry:   opts.retry,
		}
		if strings.TrimSpace(cfg.BearerToken) != "" {
			httpCfg.Auth = &httpclient.AuthConfig{Type: httpclient.AuthBearer, Token: cfg.BearerToken}
		}
		client, err = httpclient.New(httpCfg)
		if err != nil {
			return nil, fmt.Errorf("triton: create http client: %w", err)
		}
	}

	return &Provider{
		cfg:        cfg,
		client:     client,
		descriptor: descriptor(cfg, parsed),
		decider:    opts.decider,
		subject:    opts.subject,
	}, nil
}

// Descriptor documents the adapter and its network egress envelope.
func (p *Provider) Descriptor() inference.Descriptor { return p.descriptor }

// Name returns the configured component name.
func (p *Provider) Name() string { return p.cfg.Name }

// IsAvailable reports whether the underlying HTTP client is reachable.
func (p *Provider) IsAvailable(ctx context.Context) bool { return p.client.IsAvailable(ctx) }

// Execute satisfies provider.RequestResponse by forwarding to Predict.
func (p *Provider) Execute(ctx context.Context, req inference.PredictRequest) (inference.PredictResponse, error) {
	return p.Predict(ctx, req)
}

func descriptor(cfg Config, _ *url.URL) inference.Descriptor {
	return inference.Descriptor{
		Name:            cfg.Name,
		Description:     cfg.Description,
		ServingProtocol: servingProtocol,
		Capabilities:    inference.CapabilityHints{SupportsStreaming: true, SupportsBatching: true},
		Available:       true,
	}
}
