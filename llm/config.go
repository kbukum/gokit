package llm

import (
	"time"

	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/security"
)

// Config holds configuration for creating an LLM adapter.
// It is provider-agnostic — the Dialect field selects the provider mapping.
type Config struct {
	// Name identifies this adapter instance (e.g., "primary-llm", "fallback-llm").
	Name string `yaml:"name" json:"name"`

	// Dialect selects the provider mapping (e.g., "ollama", "openai", "anthropic").
	// Must match a dialect registered via RegisterDialect.
	Dialect string `yaml:"dialect" json:"dialect"`

	// BaseURL is the provider's API base URL (e.g., "http://localhost:11434").
	BaseURL string `yaml:"base_url" json:"base_url"`

	// Model is the default model to use (e.g., "gpt-4", "qwen2.5:1.5b").
	Model string `yaml:"model" json:"model"`

	// Temperature is the default sampling temperature (0.0-1.0).
	Temperature float64 `yaml:"temperature" json:"temperature"`

	// MaxTokens is the default maximum tokens for responses. 0 means provider default.
	MaxTokens int `yaml:"max_tokens" json:"max_tokens"`

	// Timeout for HTTP requests. Defaults to 120s.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// Auth configures authentication (Bearer token, API key, etc.).
	Auth *httpclient.AuthConfig `yaml:"auth" json:"auth"`

	// TLS configures TLS for the connection.
	TLS *security.TLSConfig `yaml:"tls" json:"tls"`

	// Headers are additional HTTP headers sent with every request.
	Headers map[string]string `yaml:"headers" json:"headers"`

	// Retry configures retry behavior for failed requests.
	Retry *resilience.RetryConfig `yaml:"retry" json:"retry"`

	// CircuitBreaker configures circuit breaker protection.
	CircuitBreaker *resilience.CircuitBreakerConfig `yaml:"circuit_breaker" json:"circuit_breaker"`

	// RateLimiter configures rate limiting.
	RateLimiter *resilience.RateLimiterConfig `yaml:"rate_limiter" json:"rate_limiter"`
}

// applyDefaults sets default values for unset config fields.
func (c *Config) applyDefaults() {
	if c.Timeout == 0 {
		c.Timeout = 120 * time.Second
	}
	if c.Name == "" && c.Dialect != "" {
		c.Name = c.Dialect + "-llm"
	}
}
