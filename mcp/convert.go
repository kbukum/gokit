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

	t.Title = def.Annotations.Title
	// Only attach annotations when there are actual non-title hints to convey,
	// so a round-trip doesn't mutate unset Safety into SafetyMutating.
	hasSafetyHint := def.Envelope.Safety != ""
	hasIdempotentHint := def.Annotations.IdempotentHint != nil
	hasOpenWorldHint := (def.Envelope.Network != nil && len(def.Envelope.Network.AllowList) > 0) ||
		len(def.Envelope.Filesystem) > 0 ||
		len(def.Envelope.Subprocess) > 0
	if hasSafetyHint || hasIdempotentHint || hasOpenWorldHint {
		annotations := toMCPAnnotations(def)
		t.Annotations = &annotations
	}

	return t
}

// toMCPAnnotations builds MCP wire-format annotations from Definition + Envelope.
// Read-only / destructive / open-world hints are derived from Envelope.
// Pointers are only set when the hint is explicitly intended to avoid round-trip mutation.
func toMCPAnnotations(def tool.Definition) sdkmcp.ToolAnnotations {
	ann := sdkmcp.ToolAnnotations{
		Title:          def.Annotations.Title,
		IdempotentHint: def.Annotations.IdempotentHint != nil && *def.Annotations.IdempotentHint,
	}
	switch def.Envelope.Safety {
	case tool.SafetyReadOnly:
		ann.ReadOnlyHint = true
	case tool.SafetyDestructive:
		ann.DestructiveHint = boolPtr(true)
	case tool.SafetyMutating:
		// Explicitly mutating: not read-only, not destructive.
		ann.DestructiveHint = boolPtr(false)
	}
	openWorld := (def.Envelope.Network != nil && len(def.Envelope.Network.AllowList) > 0) ||
		len(def.Envelope.Filesystem) > 0 ||
		len(def.Envelope.Subprocess) > 0
	if openWorld {
		ann.OpenWorldHint = boolPtr(true)
	}
	return ann
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

	// Convert annotations. MCP safety/open-world hints are executable wire metadata,
	// so they populate Envelope rather than internal Annotations.
	// Only set Safety when hints are explicitly present; otherwise preserve "unknown".
	title := t.Title
	if t.Annotations != nil {
		if t.Annotations.Title != "" {
			title = t.Annotations.Title
		}
		switch {
		case t.Annotations.DestructiveHint != nil && *t.Annotations.DestructiveHint:
			def.Envelope.Safety = tool.SafetyDestructive
		case t.Annotations.ReadOnlyHint:
			def.Envelope.Safety = tool.SafetyReadOnly
		case t.Annotations.DestructiveHint != nil:
			// Explicitly not destructive + not read-only → mutating.
			def.Envelope.Safety = tool.SafetyMutating
		}
		// Only set Mutating if we have explicit non-zero annotations indicating
		// the tool is neither read-only nor destructive.
		if t.Annotations.IdempotentHint {
			def.Annotations.IdempotentHint = boolPtr(true)
		}
	}
	def.Annotations.Title = title

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
