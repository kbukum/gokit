// Package llm provides a config-driven LLM adapter built on gokit's HTTP/REST foundation.
//
// The adapter works with any LLM provider (Ollama, OpenAI, Anthropic, Gemini, etc.) via the Dialect pattern
// — similar to how database/sql works with driver packages.
//
// # Architecture
//
// The llm package provides:
//   - Universal types: [CompletionRequest], [CompletionResponse], [StreamEvent], [chat.Message],
//     [Usage]
//   - [Dialect] interface: maps universal types to/from provider-specific HTTP format
//   - [Adapter]: composes gokit's REST client + a Dialect to create a complete LLM client
//   - [DialectRegistry]: explicit, isolated, thread-safe registry of dialect drivers
//   - Convenience helpers: [Complete], [CompleteStructured]
//
// # Usage
//
// Driver packages (under github.com/kbukum/gokit/llm/providers/...)
// expose a Register function that adds their dialect to a registry. Build a registry,
// register the providers you want, then create an adapter:
//
//	import (
//	    "github.com/kbukum/gokit/llm"
//	    "github.com/kbukum/gokit/llm/providers/openai"
//	)
//
//	reg := llm.NewDialectRegistry()
//	if err := openai.Register(reg); err != nil {
//	    return err
//	}
//
//	adapter, err := llm.New(reg, llm.Config{
//	    Dialect: "openai",
//	    BaseURL: "https://api.openai.com",
//	    Model:   "gpt-4o-mini",
//	})
//
//	resp, err := adapter.Execute(ctx, llm.CompletionRequest{
//	    Messages: []llm.Message{{Role: "user", Content: "Hello!"}},
//	})
//
// Or pass a dialect directly without a registry:
//
//	adapter, err := llm.NewWithDialect(myDialect, llm.Config{...})
//
// # Writing a Dialect
//
// Implement the [Dialect] interface in a driver package
// and expose a Register function that callers invoke against an explicit *DialectRegistry:
//
//	package myprovider
//
//	func Register(reg *llm.DialectRegistry) error {
//	    return reg.Register("my-provider", &Dialect{})
//	}
//
// See the Dialect interface documentation for details on each method.
//
// # Token counting
//
// [TokenCounter] is the string-level token-counting port consumed by metrics and
// cost/length estimators. Count is fallible — an exact tokenizer can surface an
// encode error at call time — so it returns (int, error) rather than a
// success-shaped count. [HeuristicTokenCounter] is the dependency-free default:
// it shares the chars/4 approximation with ai/chat, so estimates stay consistent
// across gokit and no divergent rule is introduced, and it never errors. For
// exact counts, inject a contrib counter built on a real tokenizer:
//
//   - github.com/kbukum/gokit/llm/tokenizer/tiktoken — OpenAI BPE (offline vocab)
//   - github.com/kbukum/gokit/llm/tokenizer/huggingface — any tokenizer.json
//
// Both live in their own sub-modules so their tokenizer dependencies stay out of
// core; construct one explicitly and pass it wherever a [TokenCounter] is needed.
package llm
