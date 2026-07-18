// Package tool provides type-safe tool definitions, auto-wiring from typed functions, a concurrent-safe registry, and middleware composition.
//
// A tool is: name + description + input schema + output schema + execute function. Schemas are auto-generated from Go types using [github.com/kbukum/gokit/schema].
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
//
// # Safety model
//
// Tool arguments are untrusted model output. [Registry.Call] fails closed: raw input is JSON-Schema validated ([ErrInvalidToolInput]) before authorization or any side effect, then run through the authorizer and sensitivity evaluator. Destructive tools ([SafetyDestructive]) are always human-gated, and both the sensitivity and human-approval defaults deny, so a call fails closed until an operator wires a real evaluator/approver. A per-tool [github.com/kbukum/gokit/resilience.Policy] can be attached via [Registry.WithToolPolicy] and read back with [Registry.PolicyFor].
package tool
