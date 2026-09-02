package httpclient

import (
	"fmt"
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

	// Timeout is the default request timeout. Defaults to 30s.
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// Auth configures default authentication applied to all requests.
	// Individual requests can override this.
	Auth *AuthConfig `yaml:"-" mapstructure:"-"`

	// TLS configures TLS settings for the HTTP transport.
	TLS *security.TLSConfig `yaml:"tls" mapstructure:"tls"`

	// DefaultHeaders are default headers applied to all requests.
	DefaultHeaders map[string]string `yaml:"default_headers" mapstructure:"default_headers"`

	// ResiliencePolicy configures the retry / circuit-breaker / rate-limiter
	// stack applied to buffered requests. Nil disables resilience. It is loaded
	// from the shared resilience.Policy config vocabulary.
	ResiliencePolicy *resilience.Policy `yaml:"resilience_policy" mapstructure:"resilience_policy"`

	// MaxResponseBodyBytes bounds the buffered response body read into memory by Do.
	// Defaults to 10 MiB. Streaming responses (DoStream) are not affected.
	MaxResponseBodyBytes int64 `yaml:"max_response_body_bytes" mapstructure:"max_response_body_bytes"`
}

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	if c.MaxResponseBodyBytes <= 0 {
		c.MaxResponseBodyBytes = defaultMaxResponseBytes
	}
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("httpclient: timeout must be positive")
	}
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return err
		}
	}
	return nil
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
