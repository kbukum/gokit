package anthropic

import (
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/llm"
)

// NewAdapter creates an LLM adapter configured for the Anthropic Messages API.
// It bridges the simple anthropic.Config to gokit's httpclient with proper API key auth (x-api-key header)
// and the required anthropic-version header.
//
//	adapter, err := anthropic.NewAdapter(anthropic.Config{
//	    APIKey: "sk-ant-...",
//	    Model:  "claude-sonnet-4-20250514",
//	})
func NewAdapter(cfg Config) (*llm.Adapter, error) {
	cfg.applyDefaults()

	llmCfg := llm.Config{
		Name:    "anthropic-llm",
		Dialect: "anthropic",
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
		Headers: map[string]string{
			"anthropic-version": cfg.APIVersion,
		},
	}

	if cfg.APIKey != "" {
		llmCfg.Auth = httpclient.APIKeyAuthHeader(cfg.APIKey, "x-api-key")
	}

	return llm.NewWithDialect(&Dialect{}, llmCfg)
}
