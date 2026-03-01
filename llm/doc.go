// Package llm provides a config-driven LLM adapter built on gokit's HTTP/REST foundation.
//
// The adapter works with any LLM provider (Ollama, OpenAI, Anthropic, etc.) via the
// Dialect pattern — similar to how database/sql works with driver packages.
//
// # Architecture
//
// The llm package provides:
//   - Universal types: [CompletionRequest], [CompletionResponse], [StreamChunk], [Message], [Usage]
//   - [Dialect] interface: maps universal types to/from provider-specific HTTP format
//   - [Adapter]: composes gokit's REST client + a Dialect to create a complete LLM client
//   - Dialect registry: [RegisterDialect] / [GetDialect] for config-driven dialect selection
//   - Convenience helpers: [Complete], [CompleteStructured]
//
// # Usage
//
// Import a dialect driver package for side-effect registration, then create an adapter:
//
//	import (
//	    "github.com/kbukum/gokit/llm"
//	    _ "github.com/your-org/llm-ollama"  // registers "ollama" dialect
//	)
//
//	adapter, err := llm.New(llm.Config{
//	    Dialect: "ollama",
//	    BaseURL: "http://localhost:11434",
//	    Model:   "qwen2.5:1.5b",
//	})
//
//	resp, err := adapter.Execute(ctx, llm.CompletionRequest{
//	    Messages: []llm.Message{{Role: "user", Content: "Hello!"}},
//	})
//
// Or pass a dialect directly without the global registry:
//
//	adapter, err := llm.NewWithDialect(myDialect, llm.Config{...})
//
// # Writing a Dialect
//
// Implement the [Dialect] interface and register it:
//
//	func init() {
//	    llm.RegisterDialect("my-provider", &MyDialect{})
//	}
//
// See the Dialect interface documentation for details on each method.
package llm
