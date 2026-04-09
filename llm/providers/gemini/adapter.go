package gemini

import (
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/llm"
)

// NewAdapter creates an LLM adapter configured for the Google Gemini API.
// It bridges the simple gemini.Config to gokit's httpclient with proper
// API key auth via query parameter (?key=API_KEY).
//
//	adapter, err := gemini.NewAdapter(gemini.Config{
//	    APIKey: "AIza...",
//	    Model:  "gemini-2.0-flash",
//	})
func NewAdapter(cfg Config) (*llm.Adapter, error) {
	cfg.applyDefaults()

	llmCfg := llm.Config{
		Name:    "gemini-llm",
		Dialect: "gemini",
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
	}

	if cfg.APIKey != "" {
		llmCfg.Auth = httpclient.APIKeyAuthQuery(cfg.APIKey, "key")
	}

	return llm.New(llmCfg)
}
