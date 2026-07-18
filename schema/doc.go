// Package schema provides JSON Schema generation from Go types.
//
// It wraps [github.com/invopop/jsonschema] to produce standard JSON Schema 2020-12 documents from struct tags, and exposes the result as a plain map[string]any suitable for tool definitions, MCP, and LLM APIs.
//
// Usage:
//
//	type SearchInput struct {
//	    Query    string `json:"query"    jsonschema:"required,description=Search query text"`
//	    Platform string `json:"platform" jsonschema:"enum=youtube,enum=tiktok,enum=instagram"`
//	    Limit    int    `json:"limit"    jsonschema:"minimum=1,maximum=100"`
//	}
//
//	s := schema.Generate[SearchInput]()
//	// s is a map[string]any representing the JSON Schema for SearchInput.
//
// Values are validated against schemas with [Validate], or by pre-checking a schema once with [Compile] and reusing the resulting [CompiledSchema]. Both apply [ValidationLimits] (depth, node count, and string-byte bounds) to guard against resource exhaustion from untrusted schema or value input.
package schema
