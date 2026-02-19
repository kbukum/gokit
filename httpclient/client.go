package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kbukum/gokit/httpclient/sse"
	"github.com/kbukum/gokit/resilience"
)

// Client is a configurable HTTP client with built-in auth, TLS, and resilience.
type Client struct {
	httpClient *http.Client
	config     Config
	cb         *resilience.CircuitBreaker
	rl         *resilience.RateLimiter
}

// New creates a new HTTP client with the given configuration.
func New(cfg Config) (*Client, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()

	// Apply TLS configuration
	if cfg.TLS != nil {
		tlsCfg, err := cfg.TLS.Build()
		if err != nil {
			return nil, err
		}
		if tlsCfg != nil {
			transport.TLSClientConfig = tlsCfg
		}
	}

	c := &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   cfg.Timeout,
		},
		config: cfg,
	}

	// Initialize resilience components
	if cfg.CircuitBreaker != nil {
		c.cb = resilience.NewCircuitBreaker(*cfg.CircuitBreaker)
	}
	if cfg.RateLimiter != nil {
		c.rl = resilience.NewRateLimiter(*cfg.RateLimiter)
	}

	return c, nil
}

// Do executes an HTTP request and returns the complete response.
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	if c.config.Retry != nil {
		return resilience.Retry(ctx, *c.config.Retry, func() (*Response, error) {
			return c.doOnce(ctx, req)
		})
	}
	return c.doOnce(ctx, req)
}

// DoStream executes an HTTP request and returns a streaming response.
// The caller must close the returned StreamResponse when done.
// Note: Retry is not applied to streaming requests.
func (c *Client) DoStream(ctx context.Context, req Request) (*StreamResponse, error) {
	return c.doStream(ctx, req)
}

// Unwrap returns the underlying *http.Client for advanced use cases.
func (c *Client) Unwrap() *http.Client {
	return c.httpClient
}

// doOnce executes a single HTTP request with CB and rate limiter.
func (c *Client) doOnce(ctx context.Context, req Request) (*Response, error) {
	execute := func() (*Response, error) {
		return c.executeRequest(ctx, req)
	}

	// Apply rate limiter
	if c.rl != nil {
		if err := c.rl.Wait(ctx); err != nil {
			return nil, err
		}
	}

	// Apply circuit breaker
	if c.cb != nil {
		var resp *Response
		err := c.cb.Execute(func() error {
			var execErr error
			resp, execErr = execute()
			return execErr
		})
		return resp, err
	}

	return execute()
}

// executeRequest builds and sends the HTTP request.
func (c *Client) executeRequest(ctx context.Context, req Request) (*Response, error) {
	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, NewTimeoutError(err)
		}
		return nil, NewConnectionError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewConnectionError(fmt.Errorf("read response body: %w", err))
	}

	result := &Response{
		StatusCode: resp.StatusCode,
		Headers:    flattenHeaders(resp.Header),
		Body:       body,
	}

	if classErr := ClassifyStatusCode(resp.StatusCode, body); classErr != nil {
		return result, classErr
	}

	return result, nil
}

// doStream builds and sends a streaming HTTP request.
func (c *Client) doStream(ctx context.Context, req Request) (*StreamResponse, error) {
	// Use a client without timeout for streaming — context handles cancellation.
	httpReq, err := c.buildRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	// Create a transport-only client for streaming (no global timeout)
	streamClient := &http.Client{
		Transport: c.httpClient.Transport,
	}

	resp, err := streamClient.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, NewTimeoutError(err)
		}
		return nil, NewConnectionError(err)
	}

	// Check for error status before starting to stream
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, ClassifyStatusCode(resp.StatusCode, body)
	}

	headers := flattenHeaders(resp.Header)

	// Detect SSE from content-type
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		return &StreamResponse{
			StatusCode: resp.StatusCode,
			Headers:    headers,
			SSE:        sse.NewReader(resp.Body),
			rawResp:    resp,
		}, nil
	}

	// Non-SSE streaming (ndjson, raw bytes, etc)
	return &StreamResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       resp.Body,
		rawResp:    resp,
	}, nil
}

// buildRequest constructs an *http.Request from the client config and request.
func (c *Client) buildRequest(ctx context.Context, req Request) (*http.Request, error) {
	// Resolve URL
	url := req.Path
	if c.config.BaseURL != "" && !strings.HasPrefix(req.Path, "http://") && !strings.HasPrefix(req.Path, "https://") {
		url = strings.TrimRight(c.config.BaseURL, "/") + "/" + strings.TrimLeft(req.Path, "/")
	}

	// Build body
	body, contentType, err := encodeBody(req.Body)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("encode body: %v", err))
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, body)
	if err != nil {
		return nil, NewValidationError(fmt.Sprintf("create request: %v", err))
	}

	// Apply query parameters
	if len(req.Query) > 0 {
		q := httpReq.URL.Query()
		for k, v := range req.Query {
			q.Set(k, v)
		}
		httpReq.URL.RawQuery = q.Encode()
	}

	// Apply default headers
	for k, v := range c.config.Headers {
		httpReq.Header.Set(k, v)
	}

	// Apply request-specific headers (override defaults)
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Set content-type if body present and not already set
	if body != nil && httpReq.Header.Get("Content-Type") == "" && contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}

	// Apply auth: request-level overrides client-level
	auth := c.config.Auth
	if req.Auth != nil {
		auth = req.Auth
	}
	auth.apply(httpReq)

	return httpReq, nil
}

// encodeBody converts a body value into an io.Reader and content type.
func encodeBody(body any) (io.Reader, string, error) {
	if body == nil {
		return nil, "", nil
	}
	switch v := body.(type) {
	case io.Reader:
		return v, "", nil
	case []byte:
		return bytes.NewReader(v), "", nil
	case string:
		return strings.NewReader(v), "text/plain", nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, "", err
		}
		return bytes.NewReader(data), "application/json", nil
	}
}

// flattenHeaders converts multi-value headers to single-value.
func flattenHeaders(h http.Header) map[string]string {
	result := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			result[k] = v[0]
		}
	}
	return result
}
