package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// TypedResponse wraps a response with a decoded body of type T.
type TypedResponse[T any] struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Headers are the response headers.
	Headers map[string]string
	// Data is the decoded response body.
	Data T
}

// RequestOption configures a single request.
type RequestOption func(*Request)

// WithHeader adds a header to the request.
func WithHeader(key, value string) RequestOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		r.Headers[key] = value
	}
}

// WithQueryParam adds a query parameter to the request.
func WithQueryParam(key, value string) RequestOption {
	return func(r *Request) {
		if r.Query == nil {
			r.Query = make(map[string]string)
		}
		r.Query[key] = value
	}
}

// WithRequestAuth overrides authentication for the request.
func WithRequestAuth(auth *AuthConfig) RequestOption {
	return func(r *Request) {
		r.Auth = auth
	}
}

// Get performs a GET request and decodes the JSON response into type T.
func Get[T any](a *Adapter, ctx context.Context, path string, opts ...RequestOption) (*TypedResponse[T], error) {
	return doTyped[T](a, ctx, http.MethodGet, path, nil, opts...)
}

// Post performs a POST request with a JSON body and decodes the response into type T.
func Post[T any](a *Adapter, ctx context.Context, path string, body any, opts ...RequestOption) (*TypedResponse[T], error) {
	return doTyped[T](a, ctx, http.MethodPost, path, body, opts...)
}

// Put performs a PUT request with a JSON body and decodes the response into type T.
func Put[T any](a *Adapter, ctx context.Context, path string, body any, opts ...RequestOption) (*TypedResponse[T], error) {
	return doTyped[T](a, ctx, http.MethodPut, path, body, opts...)
}

// Patch performs a PATCH request with a JSON body and decodes the response into type T.
func Patch[T any](a *Adapter, ctx context.Context, path string, body any, opts ...RequestOption) (*TypedResponse[T], error) {
	return doTyped[T](a, ctx, http.MethodPatch, path, body, opts...)
}

// Delete performs a DELETE request and decodes the JSON response into type T.
func Delete[T any](a *Adapter, ctx context.Context, path string, opts ...RequestOption) (*TypedResponse[T], error) {
	return doTyped[T](a, ctx, http.MethodDelete, path, nil, opts...)
}

// doTyped executes a typed REST request and decodes the JSON response.
func doTyped[T any](a *Adapter, ctx context.Context, method, path string, body any, opts ...RequestOption) (*TypedResponse[T], error) {
	req := Request{
		Method: method,
		Path:   path,
		Body:   body,
	}
	for _, opt := range opts {
		opt(&req)
	}

	resp, err := a.Do(ctx, req)
	if err != nil {
		// If we got a response with an error (e.g., 4xx/5xx), try to decode it
		if resp != nil {
			var data T
			if jsonErr := json.Unmarshal(resp.Body, &data); jsonErr == nil {
				return &TypedResponse[T]{
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
			return nil, fmt.Errorf("httpclient: decode response: %w", err)
		}
	}

	return &TypedResponse[T]{
		StatusCode: resp.StatusCode,
		Headers:    resp.Headers,
		Data:       data,
	}, nil
}
