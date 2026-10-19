package httpclient

import (
	"crypto/tls"
	"fmt"
	"math"
	"time"

	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
)

const (
	defaultTimeout = 30 * time.Second
	// defaultMaxResponseBytes bounds how much of a buffered (non-streaming)
	// response body the adapter reads into memory. A remote endpoint is a trust
	// boundary; capping the read stops a hostile or misbehaving server from
	// exhausting memory with an unbounded body. Streaming requests (DoStream)
	// are not buffered and so are not subject to this cap.
	defaultMaxResponseBytes = 10 << 20 // 10 MiB
)

// Config configures the HTTP adapter.
type Config struct {
	// Name identifies this adapter instance (used by provider.Provider interface).
	Name string `yaml:"name" mapstructure:"name"`

	// BaseURL is the root URL for the target service (e.g., "https://api.example.com/v1").
	// Set statically via YAML or dynamically via discovery.
	BaseURL string `yaml:"base_url" mapstructure:"base_url"`

	// Timeout bounds each buffered attempt, from sending the request to reading the body. Waiting for admission and retries are not included; bound them with ResiliencePolicy or the caller's deadline. Defaults to 30s; negative is invalid. Streams use Stream instead.
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// Stream configures finite streaming budgets and frame limits independently of buffered requests.
	Stream StreamConfig `yaml:"stream" mapstructure:"stream"`

	// Auth configures default authentication applied to all requests.
	// Individual requests can override this.
	Auth *AuthConfig `yaml:"-" mapstructure:"-"`

	// TLS configures TLS settings for the HTTP transport.
	TLS *security.TLSConfig `yaml:"tls" mapstructure:"tls"`

	// EndpointPolicy constrains every request, redirect and DNS-at-dial destination. Injected, never loaded from wire config.
	EndpointPolicy *security.EndpointPolicy `yaml:"-" mapstructure:"-"`

	// DefaultHeaders are default headers applied to all requests.
	DefaultHeaders map[string]string `yaml:"default_headers" mapstructure:"default_headers"`

	// ResiliencePolicy configures shared admission and breaker accounting. Streams hold admission until terminal cleanup and never retry. Buffered requests also use the retry policy. Nil disables resilience, not finite transport budgets.
	ResiliencePolicy *resilience.Policy `yaml:"resilience_policy" mapstructure:"resilience_policy"`

	// MaxResponseBodyBytes bounds the buffered response body read into memory by Do.
	// Defaults to 10 MiB; negative is invalid. Streaming responses (DoStream) are not affected.
	MaxResponseBodyBytes int64 `yaml:"max_response_body_bytes" mapstructure:"max_response_body_bytes"`

	// MaxConnsPerHost caps open connections to one host, and idle ones with it. Zero means no cap; negative is invalid.
	MaxConnsPerHost int `yaml:"max_conns_per_host" mapstructure:"max_conns_per_host"`
}

// ApplyDefaults fills unset (zero) fields. It leaves bad values for Validate to reject.
func (c *Config) ApplyDefaults() {
	c.Stream.ApplyDefaults()
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
	if c.MaxResponseBodyBytes == 0 {
		c.MaxResponseBodyBytes = defaultMaxResponseBytes
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	stream := c.Stream
	stream.ApplyDefaults()
	if err := stream.Validate(); err != nil {
		return err
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("httpclient: timeout must be positive")
	}
	if c.MaxResponseBodyBytes < 0 {
		return NewValidationError("max_response_body_bytes must not be negative")
	}
	if c.MaxConnsPerHost < 0 {
		return NewValidationError("max_conns_per_host must not be negative")
	}
	if c.MaxResponseBodyBytes == math.MaxInt64 {
		return NewValidationError("max_response_body_bytes must leave room for overflow detection")
	}
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return err
		}
	}
	if c.EndpointPolicy != nil {
		if c.TLS != nil && (c.TLS.SkipVerify || c.TLS.ServerName != "" || (c.TLS.MinVersion != 0 && c.TLS.MinVersion < tls.VersionTLS13)) {
			return NewValidationError("endpoint policy requires hostname verification and TLS 1.3 without a server-name override")
		}
		if c.ResiliencePolicy != nil && c.ResiliencePolicy.Retry != nil {
			return NewValidationError("endpoint policy refuses implicit request retries")
		}
	}
	return c.ResiliencePolicy.Validate()
}

// DefaultRetryConfig returns a default retry config suitable for HTTP clients.
func DefaultRetryConfig() *resilience.RetryConfig {
	cfg := resilience.DefaultRetryConfig()
	cfg.RetryIf = IsRetryable
	return &cfg
}

// DefaultCircuitBreakerConfig returns a default circuit breaker config.
func DefaultCircuitBreakerConfig(name string) *resilience.CircuitBreakerConfig {
	cfg := resilience.DefaultCircuitBreakerConfig(name)
	return &cfg
}

// DefaultRateLimiterConfig returns a default rate limiter config.
func DefaultRateLimiterConfig(name string) *resilience.RateLimiterConfig {
	cfg := resilience.DefaultRateLimiterConfig(name)
	return &cfg
}
