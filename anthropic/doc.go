// Package anthropic provides Anthropic Claude LLM dialect and embedding
// implementations for the gokit framework.
//
// This vendor module implements the llm.Dialect interface for Anthropic's
// Messages API. Import it to register the "anthropic" dialect:
//
//	import _ "github.com/kbukum/gokit/anthropic"
//
//	adapter := llm.NewAdapter("anthropic", "https://api.anthropic.com", opts...)
package anthropic
