package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/httpclient"
)

// Client is a JSON-focused REST client that wraps the base HTTP client.
// All requests use Content-Type: application/json and Accept: application/json.
type Client struct {
	http *httpclient.Client
}

// New creates a new REST client from the given config.
// JSON headers are applied automatically.
func New(cfg httpclient.Config) (*Client, error) {
	// Ensure JSON headers
	if cfg.Headers == nil {
		cfg.Headers = make(map[string]string)
	}
	if _, ok := cfg.Headers["Content-Type"]; !ok {
		cfg.Headers["Content-Type"] = "application/json"
	}
	if _, ok := cfg.Headers["Accept"]; !ok {
		cfg.Headers["Accept"] = "application/json"
	}

	c, err := httpclient.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{http: c}, nil
}

// NewFromClient creates a REST client from an existing HTTP client.
func NewFromClient(c *httpclient.Client) *Client {
	return &Client{http: c}
}

// HTTP returns the underlying HTTP client.
func (c *Client) HTTP() *httpclient.Client {
	return c.http
}

// RequestOption configures a single REST request.
type RequestOption func(*httpclient.Request)

// WithQuery adds query parameters to the request.
func WithQuery(params map[string]string) RequestOption {
	return func(r *httpclient.Request) {
		r.Query = params
	}
}

// WithHeaders adds headers to the request.
func WithHeaders(headers map[string]string) RequestOption {
	return func(r *httpclient.Request) {
		r.Headers = headers
	}
}

// WithAuth overrides authentication for the request.
func WithAuth(auth *httpclient.AuthConfig) RequestOption {
	return func(r *httpclient.Request) {
		r.Auth = auth
	}
}

// Response wraps a typed REST response.
type Response[T any] struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Headers are the response headers.
	Headers map[string]string
	// Data is the decoded response body.
	Data T
}

// Get performs a GET request and decodes the JSON response into type T.
func Get[T any](ctx context.Context, c *Client, path string, opts ...RequestOption) (*Response[T], error) {
	return do[T](ctx, c, http.MethodGet, path, nil, opts...)
}

// Post performs a POST request with a JSON body and decodes the response into type T.
func Post[T any](ctx context.Context, c *Client, path string, body any, opts ...RequestOption) (*Response[T], error) {
	return do[T](ctx, c, http.MethodPost, path, body, opts...)
}

// Put performs a PUT request with a JSON body and decodes the response into type T.
func Put[T any](ctx context.Context, c *Client, path string, body any, opts ...RequestOption) (*Response[T], error) {
	return do[T](ctx, c, http.MethodPut, path, body, opts...)
}

// Patch performs a PATCH request with a JSON body and decodes the response into type T.
func Patch[T any](ctx context.Context, c *Client, path string, body any, opts ...RequestOption) (*Response[T], error) {
	return do[T](ctx, c, http.MethodPatch, path, body, opts...)
}

// Delete performs a DELETE request and decodes the response into type T.
func Delete[T any](ctx context.Context, c *Client, path string, opts ...RequestOption) (*Response[T], error) {
	return do[T](ctx, c, http.MethodDelete, path, nil, opts...)
}

// do executes a REST request and decodes the JSON response.
func do[T any](ctx context.Context, c *Client, method, path string, body any, opts ...RequestOption) (*Response[T], error) {
	req := httpclient.Request{
		Method: method,
		Path:   path,
		Body:   body,
	}
	for _, opt := range opts {
		opt(&req)
	}

	resp, err := c.http.Do(ctx, req)
	if err != nil {
		// If we got a response with an error (e.g., 4xx/5xx), try to decode it
		if resp != nil {
			var data T
			if jsonErr := json.Unmarshal(resp.Body, &data); jsonErr == nil {
				return &Response[T]{
					StatusCode: resp.StatusCode,
					Headers:    resp.Headers,
					Data:       data,
				}, err
			}
		}
		return nil, err
	}

	var data T
	if len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, &data); err != nil {
			return nil, fmt.Errorf("httpclient/rest: decode response: %w", err)
		}
	}

	return &Response[T]{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Data:       data,
	}, nil
}
