package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/httpclient/rest"
)

// Sentinel errors.
var (
	ErrNoDialect    = errors.New("llm: dialect is required")
	ErrNoSSEReader  = errors.New("llm: expected SSE stream but got no SSE reader")
	ErrNoStreamBody = errors.New("llm: expected stream body but got nil")
)

// Adapter is a config-driven LLM client that works with any provider via the Dialect pattern.
//
// It composes gokit's REST client (which wraps the HTTP adapter) with a Dialect that handles provider-specific request/response mapping. This gives you:
//   - TLS, auth, resilience, timeout from the HTTP adapter
//   - JSON encoding/decoding from the REST client
//   - Provider-specific mapping from the Dialect
//
// Adapter implements:
//   - provider.RequestResponse[CompletionRequest, CompletionResponse]
//   - provider.Streamable[CompletionRequest, CompletionResponse, StreamEvent]
//   - provider.Closeable
type Adapter struct {
	rest      *rest.Client
	dialect   Dialect
	model     string
	temp      float64
	maxTokens int
}

// New creates an LLM adapter from config using the supplied dialect registry. The config's Dialect field must match a name registered in `registry`. Pass an explicit registry built via [NewDialectRegistry] and populated via the driver packages' Register functions.
func New(registry *DialectRegistry, cfg Config) (*Adapter, error) {
	if registry == nil {
		return nil, fmt.Errorf("llm: dialect registry is nil")
	}
	cfg.applyDefaults()

	dialect, err := registry.Get(cfg.Dialect)
	if err != nil {
		return nil, err
	}

	return newAdapter(dialect, cfg)
}

// NewWithDialect creates an LLM adapter with an explicit dialect instance. Use this when you don't want to rely on the global dialect registry.
func NewWithDialect(dialect Dialect, cfg Config) (*Adapter, error) {
	if dialect == nil {
		return nil, ErrNoDialect
	}
	cfg.applyDefaults()
	if cfg.Name == "" {
		cfg.Name = dialect.Name() + "-llm"
	}
	return newAdapter(dialect, cfg)
}

func newAdapter(dialect Dialect, cfg Config) (*Adapter, error) {
	restCfg := httpclient.Config{
		BaseURL:        cfg.BaseURL,
		Name:           cfg.Name,
		Timeout:        cfg.Timeout,
		Auth:           cfg.Auth,
		TLS:            cfg.TLS,
		Headers:        cfg.Headers,
		Retry:          cfg.Retry,
		CircuitBreaker: cfg.CircuitBreaker,
		RateLimiter:    cfg.RateLimiter,
	}
	client, err := rest.New(restCfg)
	if err != nil {
		return nil, fmt.Errorf("llm: create rest client: %w", err)
	}

	return &Adapter{
		rest:      client,
		dialect:   dialect,
		model:     cfg.Model,
		temp:      cfg.Temperature,
		maxTokens: cfg.MaxTokens,
	}, nil
}

// --- provider.Provider interface ---

// Name returns the adapter name.
func (a *Adapter) Name() string { return a.rest.Name() }

// IsAvailable checks if the LLM provider is reachable. Uses the dialect's health endpoint if available, otherwise delegates to the REST client.
func (a *Adapter) IsAvailable(ctx context.Context) bool {
	if hp := a.dialect.HealthPath(); hp != "" {
		_, err := rest.Get[json.RawMessage](ctx, a.rest, hp)
		return err == nil
	}
	return a.rest.IsAvailable(ctx)
}

// --- provider.Closeable interface ---

// Close releases resources.
func (a *Adapter) Close(ctx context.Context) error { return a.rest.Close(ctx) }

// --- provider.RequestResponse[CompletionRequest, CompletionResponse] interface ---

// Execute sends a completion request and returns the full response.
func (a *Adapter) Execute(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	a.applyDefaults(&req)

	body, err := a.dialect.BuildRequest(req)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("llm: build request: %w", err)
	}

	resp, err := rest.Post[json.RawMessage](ctx, a.rest, a.dialect.ChatPath(), body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("llm: execute: %w", err)
	}

	result, err := a.dialect.ParseResponse(resp.Data)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("llm: parse response: %w", err)
	}
	return *result, nil
}

// --- provider.Streamable streaming ---

// Stream sends a completion request and returns canonical stream events. The channel is closed when the stream ends or an error occurs.
func (a *Adapter) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamEvent, error) {
	chunkCh, model, streamCtx, cancel, err := a.streamChunks(ctx, req)
	if err != nil {
		return nil, err
	}
	return streamEventsFromChunks(streamCtx, chunkCh, model, cancel), nil
}

// streamChunks starts the upstream stream and returns the chunk channel, the cancelable stream context, and a cancel func that tears down the producer goroutine and underlying connection. To stop early, callers cancel the context passed to Stream (or the returned streamCtx); every send in the pipeline selects on that context so no goroutine blocks on an abandoned channel.
func (a *Adapter) streamChunks(ctx context.Context, req CompletionRequest) (chunkCh <-chan streamChunk, model string, streamCtx context.Context, cancel context.CancelFunc, err error) {
	a.applyDefaults(&req)
	req.Stream = true

	body, err := a.dialect.BuildRequest(req)
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("llm: build stream request: %w", err)
	}

	streamCtx, cancel = context.WithCancel(ctx)
	streamResp, err := a.rest.HTTP().DoStream(streamCtx, httpclient.Request{
		Method: http.MethodPost,
		Path:   a.dialect.ChatPath(),
		Body:   body,
	})
	if err != nil {
		cancel()
		return nil, "", nil, nil, fmt.Errorf("llm: stream: %w", err)
	}

	ch := make(chan streamChunk, 1)
	go a.readStream(streamCtx, streamResp, ch)
	return ch, req.Model, streamCtx, cancel, nil
}

// --- Accessors ---

// Dialect returns the dialect used by this adapter.
func (a *Adapter) Dialect() Dialect { return a.dialect }

// REST returns the underlying REST client for advanced use cases.
func (a *Adapter) REST() *rest.Client { return a.rest }

// --- internal ---

func (a *Adapter) applyDefaults(req *CompletionRequest) {
	if req.Model == "" {
		req.Model = a.model
	}
	if req.Temperature == nil && a.temp != 0 {
		req.Temperature = &a.temp
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = a.maxTokens
	}
}
