// Package ollama is a first-class LLM provider for Ollama (https://ollama.com), which exposes an OpenAI-compatible /v1/chat/completions endpoint at http://localhost:11434 by default.
//
// Per locked decision D3, ollama is a first-class provider for naming honesty and discoverability, while reusing the OpenAI wire dialect at the code level (DRY at code, distinct at API). Future native /api/* endpoints (model auto-pull, num_ctx, keep_alive) can be added without changing the public surface.
//
// Usage:
//
//	registry := llm.NewDialectRegistry()
//	if err := ollama.Register(registry); err != nil { /* ... */ }
//	adapter, err := llm.New(registry, llm.Config{
//	    Dialect: ollama.DialectName,
//	    BaseURL: ollama.DefaultBaseURL,
//	    Model:   "llama3.2",
//	})
package ollama

import (
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/llm/providers/openai"
)

// DialectName is the registry key for the Ollama dialect.
const DialectName = "ollama"

// DefaultBaseURL is Ollama's default local listen address.
const DefaultBaseURL = "http://localhost:11434"

// Dialect implements [llm.Dialect] for Ollama. It reuses OpenAI's wire shape (Ollama exposes an OpenAI-compatible endpoint) but reports its own Name() so observability, configuration, and routing remain honest.
type Dialect struct {
	openai.Dialect
}

// Name returns "ollama" (overrides the embedded openai dialect's name).
func (Dialect) Name() string { return DialectName }

// Register installs the Ollama dialect into the supplied registry. Call once at application startup before invoking [llm.New].
func Register(registry *llm.DialectRegistry) error {
	return registry.Register(DialectName, &Dialect{})
}
