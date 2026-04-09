package anthropic

const (
	defaultBaseURL = "https://api.anthropic.com"
	defaultModel   = "claude-sonnet-4-20250514"
	defaultVersion = "2023-06-01"
)

// Config holds Anthropic-specific settings.
type Config struct {
	// BaseURL is the API base URL. Defaults to https://api.anthropic.com.
	BaseURL string `json:"base_url,omitempty" yaml:"base_url"`
	// APIKey is the Anthropic API key.
	APIKey string `json:"api_key,omitempty" yaml:"api_key"`
	// Model is the model identifier (e.g., "claude-sonnet-4-20250514").
	Model string `json:"model,omitempty" yaml:"model"`
	// APIVersion is the Anthropic API version header. Defaults to "2023-06-01".
	APIVersion string `json:"api_version,omitempty" yaml:"api_version"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:    defaultBaseURL,
		Model:      defaultModel,
		APIVersion: defaultVersion,
	}
}

func (c *Config) applyDefaults() {
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if c.Model == "" {
		c.Model = defaultModel
	}
	if c.APIVersion == "" {
		c.APIVersion = defaultVersion
	}
}
