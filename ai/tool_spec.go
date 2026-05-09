package ai

// ToolSpec is the lean LLM-layer view of a tool: name, description, and input
// schema. The richer tool.Definition from package tool converts to this shape via
// tool.Definition.ToolSpec(), letting llm consumers describe tools to providers
// without coupling the llm layer to the tool layer (D13: llm must not import
// tool).
type ToolSpec struct {
	// Name is the unique tool identifier.
	Name string `json:"name"`
	// Description explains what the tool does.
	Description string `json:"description,omitempty"`
	// InputSchema is a JSON Schema describing the tool input.
	InputSchema map[string]any `json:"input_schema,omitempty"`
}
