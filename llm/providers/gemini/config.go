package gemini

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com"
	defaultModel   = "gemini-2.0-flash"
)

// Config holds Gemini-specific configuration.
type Config struct {
	// BaseURL is the API base URL. Defaults to Google's Generative AI endpoint. Override for Vertex AI
	// or proxies.
	BaseURL string `json:"base_url,omitempty" yaml:"base_url"`

	// APIKey is the Google AI API key.
	APIKey string `json:"api_key,omitempty" yaml:"api_key"`

	// Model is the Gemini model to use (e.g., "gemini-2.0-flash", "gemini-2.5-pro").
	Model string `json:"model,omitempty" yaml:"model"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL: defaultBaseURL,
		Model:   defaultModel,
	}
}

func (c *Config) applyDefaults() {
	if c.BaseURL == "" {
		c.BaseURL = defaultBaseURL
	}
	if c.Model == "" {
		c.Model = defaultModel
	}
}
