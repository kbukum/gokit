package security

import (
	"github.com/kbukum/gokit/schema"
	"github.com/kbukum/gokit/tool"
)

// ResultSizeBytes reports the serialized size of a tool result, preferring the structured Output payload and falling back to the text content.
func ResultSizeBytes(result *tool.Result) int {
	switch {
	case result == nil:
		return 0
	case len(result.Output) > 0:
		return len(result.Output)
	default:
		return len([]byte(result.Text()))
	}
}

// ValidateOutput validates a tool result against its declared output schema. Missing schemas and error results are treated as valid (nothing to check).
func ValidateOutput(def tool.Definition, result *tool.Result) schema.ValidationResult {
	if def.OutputSchema == nil || result == nil || result.IsError {
		return schema.ValidationResult{Valid: true}
	}
	if len(result.Output) > 0 {
		return schema.Validate(def.OutputSchema, result.Output)
	}
	return schema.Validate(def.OutputSchema, result.Text())
}

// FirstValidationError returns the first validation error message, or a generic fallback when the slice is empty (a validator that reports Valid=false must populate Errors, but the guard avoids out-of-bounds access).
func FirstValidationError(errs []schema.ValidationError) string {
	if len(errs) == 0 {
		return "validation failed"
	}
	return errs[0].Error()
}
