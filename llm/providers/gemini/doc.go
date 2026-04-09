// Package gemini provides a Google Gemini LLM dialect for gokit.
// Import this package to register the "gemini" dialect automatically,
// or use [NewAdapter] directly.
//
// Quick start:
//
//	import _ "github.com/kbukum/gokit/llm/providers/gemini" // registers "gemini" dialect
//
//	adapter, err := gemini.NewAdapter(gemini.Config{
//	    APIKey: "AIza...",
//	    Model:  "gemini-2.0-flash",
//	})
//
// The Gemini API uses API key authentication via query parameter (?key=API_KEY),
// which is handled automatically by [NewAdapter].
package gemini
