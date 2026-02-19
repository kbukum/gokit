package httpclient

import (
	"fmt"
	"time"

	"github.com/kbukum/gokit/resilience"
)

const (
	defaultTimeout = 30 * time.Second
)

// Config configures the HTTP client.
type Config struct {
	// BaseURL is the base URL prepended to all request paths.
	BaseURL string `yaml:"base_url" mapstructure:"base_url"`

	// Timeout is the default request timeout. Defaults to 30s.
	Timeout time.Duration `yaml:"timeout" mapstructure:"timeout"`

	// Auth configures default authentication applied to all requests.
	// Individual requests can override this.
	Auth *AuthConfig `yaml:"-" mapstructure:"-"`

	// TLS configures TLS settings for the HTTP transport.
	TLS *TLSConfig `yaml:"tls" mapstructure:"tls"`

	// Headers are default headers applied to all requests.
	Headers map[string]string `yaml:"headers" mapstructure:"headers"`

	// Retry configures retry behavior. Nil disables retry.
	Retry *resilience.RetryConfig `yaml:"-" mapstructure:"-"`

	// CircuitBreaker configures circuit breaker behavior. Nil disables it.
	CircuitBreaker *resilience.CircuitBreakerConfig `yaml:"-" mapstructure:"-"`

	// RateLimiter configures rate limiting. Nil disables it.
	RateLimiter *resilience.RateLimiterConfig `yaml:"-" mapstructure:"-"`
}

// ApplyDefaults fills in zero-value fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
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
