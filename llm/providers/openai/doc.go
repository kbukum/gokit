// Package openai provides an OpenAI-compatible LLM dialect and embedding
// provider for gokit. Import this package to register the "openai" dialect
// automatically, or use [NewAdapter] / [NewEmbeddingProvider] directly.
//
// Works with OpenAI, Azure OpenAI, Ollama, vLLM, llama.cpp, LM Studio,
// Together, Groq, and any server that exposes the /v1/chat/completions
// and /v1/embeddings endpoints.
//
// Quick start:
//
//	import _ "github.com/kbukum/gokit/llm/providers/openai" // registers "openai" dialect
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
package openai
