package triton

import (
	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/resilience"
)

// Config configures the Triton KServe v2 HTTP adapter.
type Config struct {
	Name           string            `json:"name,omitempty"`
	Description    string            `json:"description,omitempty"`
	BaseURL        string            `json:"base_url"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	BearerToken    string            `json:"bearer_token,omitempty"`
}

// Option injects lower-layer dependencies into Provider.
type Option func(*providerOptions)

type providerOptions struct {
	httpClient *httpclient.Adapter
	retry      *resilience.RetryConfig
	decider    authz.Decider
	subject    authz.Subject
}

// WithHTTPClient injects a configured httpclient.Adapter.
func WithHTTPClient(client *httpclient.Adapter) Option {
	return func(opts *providerOptions) { opts.httpClient = client }
}

// WithRetry injects a resilience retry policy without inventing local retry loops.
func WithRetry(retry resilience.RetryConfig) Option {
	return func(opts *providerOptions) { opts.retry = &retry }
}

// WithDecider injects an optional authz decider. Nil means open.
func WithDecider(decider authz.Decider, subject authz.Subject) Option {
	return func(opts *providerOptions) {
		opts.decider = decider
		opts.subject = subject
	}
}
