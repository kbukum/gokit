// Package anthropic provides an Anthropic Claude LLM dialect for gokit. Use
// [Register] to install the "anthropic" dialect into a [llm.DialectRegistry]
// explicitly, or use [NewAdapter] directly. There are no init() side
// effects; registration is always explicit (D-cross-cutting #1).
//
// Quick start:
//
//	registry := llm.NewDialectRegistry()
//	if err := anthropic.Register(registry); err != nil { /* handle */ }
//
//	adapter, err := anthropic.NewAdapter(anthropic.Config{
//	    APIKey: "sk-ant-...",
//	    Model:  "claude-sonnet-4-20250514",
//	})
package anthropic
