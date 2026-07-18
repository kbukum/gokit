package openai

import (
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/llm"
)

// NewAdapter creates an LLM adapter configured for an OpenAI-compatible API.
// It bridges the simple openai.Config to gokit's httpclient with proper Bearer token auth,
// timeouts, and resilience.
//
// Works with OpenAI, Ollama, vLLM, llama.cpp, LM Studio, Together, Groq —
// any server exposing /v1/chat/completions.
//
//	adapter, err := openai.NewAdapter(openai.Config{
//	    APIKey: "sk-...",
//	    Model:  "gpt-4o",
//	})
//
//	// Or for local Ollama:
//	adapter, err := openai.NewAdapter(openai.Config{
//	    BaseURL: "http://localhost:11434/v1",
//	    Model:   "llama3",
//	})
func NewAdapter(cfg Config) (*llm.Adapter, error) {
	cfg.applyDefaults()

	llmCfg := llm.Config{
		Name:    "openai-llm",
		Dialect: "openai",
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
	}

	if cfg.APIKey != "" {
		llmCfg.Auth = httpclient.BearerAuth(cfg.APIKey)
	}

	return llm.NewWithDialect(&Dialect{}, llmCfg)
}
