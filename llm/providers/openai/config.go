package openai

// Config holds settings for connecting to an OpenAI-compatible API.
type Config struct {
	// BaseURL is the API root (e.g., "https://api.openai.com/v1").
	// Defaults to "https://api.openai.com/v1" if empty.
	BaseURL string `json:"base_url,omitempty" yaml:"base_url"`
	// APIKey is the bearer token. Empty disables authentication.
	APIKey string `json:"api_key,omitempty" yaml:"api_key"`
	// Model is the default model name (e.g., "gpt-4o").
	Model string `json:"model,omitempty" yaml:"model"`
	// EmbeddingModel is the default model for embeddings (e.g., "text-embedding-3-small").
	EmbeddingModel string `json:"embedding_model,omitempty" yaml:"embedding_model"`
	// EmbeddingDimensions is the expected embedding vector size.
	EmbeddingDimensions int `json:"embedding_dimensions,omitempty" yaml:"embedding_dimensions"`
}

// DefaultConfig returns sensible defaults for the OpenAI API.
func DefaultConfig() Config {
	return Config{
		BaseURL:             "https://api.openai.com/v1",
		Model:               "gpt-4o",
		EmbeddingModel:      "text-embedding-3-small",
		EmbeddingDimensions: 1536,
	}
}

func (c *Config) applyDefaults() {
	d := DefaultConfig()
	if c.BaseURL == "" {
		c.BaseURL = d.BaseURL
	}
	if c.Model == "" {
		c.Model = d.Model
	}
	if c.EmbeddingModel == "" {
		c.EmbeddingModel = d.EmbeddingModel
	}
	if c.EmbeddingDimensions == 0 {
		c.EmbeddingDimensions = d.EmbeddingDimensions
	}
}
