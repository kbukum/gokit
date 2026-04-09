package anthropic

const (
	defaultBaseURL = "https://api.anthropic.com"
	defaultModel   = "claude-sonnet-4-20250514"
	defaultVersion = "2023-06-01"
)

// Config holds Anthropic-specific settings.
type Config struct {
	BaseURL    string // API base URL (default: https://api.anthropic.com)
	APIKey     string // Anthropic API key
	Model      string // Model identifier (default: claude-sonnet-4-20250514)
	APIVersion string // Anthropic API version header (default: 2023-06-01)
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
