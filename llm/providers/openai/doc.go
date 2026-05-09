// Package openai provides an OpenAI-compatible LLM dialect and embedding
// provider for gokit. Use [Register] to install the "openai" dialect into
// a [llm.DialectRegistry] explicitly, or use [NewAdapter] /
// [NewEmbeddingProvider] directly. There are no init() side effects;
// registration is always explicit (D-cross-cutting #1).
//
// Works with OpenAI, Azure OpenAI, Ollama, vLLM, llama.cpp, LM Studio,
// Together, Groq, and any server that exposes the /v1/chat/completions
// and /v1/embeddings endpoints.
//
// Quick start:
//
//	registry := llm.NewDialectRegistry()
//	if err := openai.Register(registry); err != nil { /* handle */ }
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
