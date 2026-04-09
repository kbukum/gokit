// Package tool provides type-safe tool definitions, auto-wiring from typed
// functions, a concurrent-safe registry, and middleware composition.
//
// A tool is: name + description + input schema + output schema + execute function.
// Schemas are auto-generated from Go types using [github.com/kbukum/gokit/schema].
//
// Quick start — create a tool from a typed function:
//
//	type SearchInput struct {
//	    Query string `json:"query" jsonschema:"required,description=Search text"`
//	}
//	type SearchOutput struct {
//	    Items []Item `json:"items"`
//	}
//
//	searchTool := tool.FromFunc("search", "Search content", doSearch)
//
// Register and use:
//
//	registry := tool.NewRegistry()
//	registry.Register(searchTool.AsCallable())
//	result, err := registry.Call(ctx, "search", json.RawMessage(`{"query":"hello"}`))
package tool
