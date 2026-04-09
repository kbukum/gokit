// Package anthropic provides an Anthropic Claude LLM dialect for gokit.
// Import this package to register the "anthropic" dialect automatically,
// or use [NewAdapter] directly.
//
// Quick start:
//
//	import _ "github.com/kbukum/gokit/llm/providers/anthropic" // registers "anthropic" dialect
//
//	adapter, err := anthropic.NewAdapter(anthropic.Config{
//	    APIKey: "sk-ant-...",
//	    Model:  "claude-sonnet-4-20250514",
//	})
package anthropic
