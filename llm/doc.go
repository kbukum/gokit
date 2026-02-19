// Package llm defines the provider interface and common types
// for interacting with large language model backends.
//
// It follows gokit's provider pattern with a pluggable registry for
// runtime-selectable backends supporting completion, structured output,
// and streaming.
//
// # Backends
//
//   - llm/ollama: Ollama local LLM inference
//
// # Usage
//
//	reg := llm.NewRegistry()
//	reg.Register("ollama", ollamaProvider)
//	resp, err := reg.Get("ollama").Complete(ctx, req)
package llm
