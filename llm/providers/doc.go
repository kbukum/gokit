// Package providers is the root of the gokit LLM provider modules.
//
// This module consolidates all vendor-specific LLM integrations (OpenAI, Anthropic, Gemini) into a single module with shared utilities.
//
// Import vendor sub-packages to register their dialects:
//
//	import _ "github.com/kbukum/gokit/llm/providers/openai"    // registers "openai" dialect
//	import _ "github.com/kbukum/gokit/llm/providers/anthropic" // registers "anthropic" dialect
//	import _ "github.com/kbukum/gokit/llm/providers/gemini"    // registers "gemini" dialect
//
// Or use the vendor-specific NewAdapter() factories for simpler setup:
//
//	adapter, err := openai.NewAdapter(openai.Config{APIKey: "sk-..."})
//	adapter, err := anthropic.NewAdapter(anthropic.Config{APIKey: "sk-ant-..."})
//	adapter, err := gemini.NewAdapter(gemini.Config{APIKey: "AIza..."})
package providers
