package mcp

import (
	"encoding/json"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kbukum/gokit/schema"
	"github.com/kbukum/gokit/tool"
)

// definitionToMCPTool converts a kit tool.Definition to an MCP Tool.
func definitionToMCPTool(def tool.Definition) *sdkmcp.Tool {
	t := &sdkmcp.Tool{
		Name:        def.Name,
		Description: def.Description,
	}

	// Convert input schema (schema.JSON is map[string]any — MCP accepts any)
	if def.InputSchema != nil {
		t.InputSchema = def.InputSchema
	}
	if def.OutputSchema != nil {
		t.OutputSchema = def.OutputSchema
	}

	// Convert annotations
	if def.Annotations != nil {
		t.Title = def.Annotations.Title
		t.Annotations = &sdkmcp.ToolAnnotations{
			Title:          def.Annotations.Title,
			ReadOnlyHint:   def.ReadOnly,
			OpenWorldHint:  def.Annotations.OpenWorldHint,
			IdempotentHint: def.Annotations.IdempotentHint != nil && *def.Annotations.IdempotentHint,
		}
		if def.Annotations.DestructiveHint != nil {
			t.Annotations.DestructiveHint = def.Annotations.DestructiveHint
		}
	} else if def.ReadOnly {
		t.Annotations = &sdkmcp.ToolAnnotations{ReadOnlyHint: true}
	}

	return t
}

// mcpToolToDefinition converts an MCP Tool to a kit tool.Definition.
func mcpToolToDefinition(t *sdkmcp.Tool) tool.Definition {
	def := tool.Definition{
		Name:        t.Name,
		Description: t.Description,
	}

	// Convert input schema
	if t.InputSchema != nil {
		if m, ok := toSchemaJSON(t.InputSchema); ok {
			def.InputSchema = m
		}
	}
	if t.OutputSchema != nil {
		if m, ok := toSchemaJSON(t.OutputSchema); ok {
			def.OutputSchema = m
		}
	}

	// Convert annotations
	if t.Annotations != nil {
		def.ReadOnly = t.Annotations.ReadOnlyHint
		def.Annotations = &tool.Annotations{
			Title:           t.Annotations.Title,
			OpenWorldHint:   t.Annotations.OpenWorldHint,
			DestructiveHint: t.Annotations.DestructiveHint,
		}
		if t.Annotations.IdempotentHint {
			def.Annotations.IdempotentHint = boolPtr(true)
		}
	}

	return def
}

// toSchemaJSON converts an any value to schema.JSON.
func toSchemaJSON(v any) (schema.JSON, bool) {
	switch val := v.(type) {
	case map[string]any:
		return val, true
	case json.RawMessage:
		var m map[string]any
		if err := json.Unmarshal(val, &m); err == nil {
			return m, true
		}
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err == nil {
			return m, true
		}
	}
	return nil, false
}

// resultToMCPResult converts a kit tool.Result to an MCP CallToolResult.
func resultToMCPResult(r *tool.Result) *sdkmcp.CallToolResult {
	result := &sdkmcp.CallToolResult{
		IsError: r.IsError,
	}

	// Use Content text, falling back to Output JSON
	text := r.Text()
	if text != "" {
		result.Content = []sdkmcp.Content{
			&sdkmcp.TextContent{Text: text},
		}
	}

	// Set structured content from Output if available
	if r.Output != nil {
		var structured any
		if err := json.Unmarshal(r.Output, &structured); err == nil {
			result.StructuredContent = structured
		}
	}

	return result
}

// mcpResultToResult converts an MCP CallToolResult to a kit tool.Result.
func mcpResultToResult(r *sdkmcp.CallToolResult) *tool.Result {
	result := &tool.Result{
		IsError: r.IsError,
	}

	// Extract text from content blocks
	for _, c := range r.Content {
		if tc, ok := c.(*sdkmcp.TextContent); ok {
			if result.Content != "" {
				result.Content += "\n"
			}
			result.Content += tc.Text
		}
	}

	// Use structured content as Output if available
	if r.StructuredContent != nil {
		if data, err := json.Marshal(r.StructuredContent); err == nil {
			result.Output = data
		}
	} else if result.Content != "" {
		// Try to use content as JSON output
		if json.Valid([]byte(result.Content)) {
			result.Output = json.RawMessage(result.Content)
		}
	}

	return result
}

func boolPtr(v bool) *bool { return &v }
