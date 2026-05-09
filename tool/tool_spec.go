package tool

import "github.com/kbukum/gokit/ai"

// ToolSpec converts a tool.Definition into an ai.ToolSpec so downstream code
// (llm, agent, inference) can pass tool specifications to LLM providers without
// depending on package tool directly.
func (d Definition) ToolSpec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        d.Name,
		Description: d.Description,
		InputSchema: d.InputSchema,
	}
}
