// Package gemini provides a Google Gemini LLM dialect for gokit.
// Use [Register] to install the "gemini" dialect into a [llm.DialectRegistry] explicitly,
// or use [NewAdapter] directly. There are no init() side effects;
// registration is always explicit (D-cross-cutting #1).
//
// Quick start:
//
//	registry := llm.NewDialectRegistry()
//	if err := gemini.Register(registry); err != nil { /* handle */ }
//
//	adapter, err := gemini.NewAdapter(gemini.Config{
//	    APIKey: "AIza...",
//	    Model:  "gemini-2.0-flash",
//	})
//
// The Gemini API uses API key authentication via the x-goog-api-key header,
// which is handled automatically by [NewAdapter].
package gemini
