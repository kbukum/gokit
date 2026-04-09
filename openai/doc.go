// Package openai provides an OpenAI-compatible LLM dialect and embedding
// provider for gokit. Import this package to register the "openai" dialect
// automatically, or use [NewDialect] / [NewEmbeddingProvider] directly.
//
// Works with OpenAI, Azure OpenAI, vLLM, llama.cpp, and any server that
// exposes the /v1/chat/completions and /v1/embeddings endpoints.
package openai
