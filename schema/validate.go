package schema

import (
	"encoding/json"
	"fmt"
)

// ValidationError describes a single validation failure.
type ValidationError struct {
	// Path is the JSON pointer to the invalid field (e.g., "/query", "/items/0").
	Path string `json:"path"`
	// Message describes what's wrong.
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationResult holds the outcome of validating a value against a schema.
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

func invalidResult(message string) ValidationResult {
	return ValidationResult{Valid: false, Errors: []ValidationError{{Message: message}}}
}

// CompiledSchema is a JSON Schema document that has been checked against structural limits and is ready to validate values repeatedly without re-inspecting the schema itself.
type CompiledSchema struct {
	schema JSON
	limits ValidationLimits
}

// Compile validates a schema document against the default structural limits and returns a reusable CompiledSchema. A nil schema compiles to a validator that accepts any value.
func Compile(s JSON) (*CompiledSchema, error) {
	return CompileWithLimits(s, DefaultLimits())
}

// CompileWithLimits is like Compile but applies caller-supplied structural limits to the schema document.
func CompileWithLimits(s JSON, limits ValidationLimits) (*CompiledSchema, error) {
	if s != nil {
		if err := limits.check("schema", s); err != nil {
			return nil, err
		}
	}
	return &CompiledSchema{schema: s, limits: limits}, nil
}

// Validate checks a JSON-serializable value against the compiled schema, enforcing the compiled structural limits on the value before inspection.
//
// The value parameter is a documented opaque-value exception to the no-any rule: it accepts any JSON-serializable Go value (including json.RawMessage or a []byte JSON payload).
func (c *CompiledSchema) Validate(value any) ValidationResult {
	if c.schema == nil {
		return ValidationResult{Valid: true}
	}
	if value == nil {
		return invalidResult("value is nil")
	}

	data, err := normalize(value)
	if err != nil {
		return invalidResult(err.Error())
	}

	if err := c.limits.check("value", data); err != nil {
		return invalidResult(err.Error())
	}

	var errs []ValidationError
	validateValue(c.schema, data, "", &errs)
	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

// Validate checks a value against a JSON Schema and returns validation results. It compiles the schema with default limits on each call; prefer Compile plus CompiledSchema.Validate when validating many values against one schema.
//
//	result := schema.Validate(mySchema, input)
//	if !result.Valid {
//	    for _, err := range result.Errors {
//	        log.Printf("validation error at %s: %s", err.Path, err.Message)
//	    }
//	}
func Validate(s JSON, value any) ValidationResult {
	compiled, err := Compile(s)
	if err != nil {
		return invalidResult(err.Error())
	}
	return compiled.Validate(value)
}

// normalize converts an arbitrary JSON-serializable value into the generic representation (map[string]any, []any, string, float64, bool, nil) used by the validator.
func normalize(value any) (any, error) {
	var raw []byte
	switch v := value.(type) {
	case json.RawMessage:
		raw = v
	case []byte:
		raw = v
	default:
		b, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("cannot serialize value: %w", err)
		}
		raw = b
	}
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return data, nil
}

// validateValue recursively validates a value against a schema.
func validateValue(s JSON, value any, path string, errs *[]ValidationError) {
	schemaType, _ := s["type"].(string)

	switch schemaType {
	case "object":
		validateObject(s, value, path, errs)
	case "array":
		validateArray(s, value, path, errs)
	case "string":
		validateString(s, value, path, errs)
	case "number", "integer":
		validateNumber(s, schemaType, value, path, errs)
	case "boolean":
		if _, ok := value.(bool); !ok {
			*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected boolean, got %T", value)})
		}
	}
}

func validateObject(s JSON, value any, path string, errs *[]ValidationError) {
	obj, ok := value.(map[string]any)
	if !ok {
		if value == nil {
			obj = map[string]any{}
		} else {
			*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected object, got %T", value)})
			return
		}
	}

	if required, ok := s["required"].([]any); ok {
		for _, r := range required {
			name, _ := r.(string)
			if _, exists := obj[name]; !exists {
				fieldPath := name
				if path != "" {
					fieldPath = path + "/" + name
				}
				*errs = append(*errs, ValidationError{Path: fieldPath, Message: "required field missing"})
			}
		}
	}

	if properties, ok := s["properties"].(map[string]any); ok {
		for key, propSchema := range properties {
			if ps, ok := propSchema.(map[string]any); ok {
				if val, exists := obj[key]; exists {
					validateValue(ps, val, path+"/"+key, errs)
				}
			}
		}
	}
}

func validateArray(s JSON, value any, path string, errs *[]ValidationError) {
	arr, ok := value.([]any)
	if !ok {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected array, got %T", value)})
		return
	}
	if items, ok := s["items"].(map[string]any); ok {
		for i, item := range arr {
			validateValue(items, item, fmt.Sprintf("%s/%d", path, i), errs)
		}
	}
	if minItems, ok := s["minItems"].(float64); ok && float64(len(arr)) < minItems {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("array has %d items, minimum is %d", len(arr), int(minItems))})
	}
	if maxItems, ok := s["maxItems"].(float64); ok && float64(len(arr)) > maxItems {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("array has %d items, maximum is %d", len(arr), int(maxItems))})
	}
}

func validateString(s JSON, value any, path string, errs *[]ValidationError) {
	str, ok := value.(string)
	if !ok {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected string, got %T", value)})
		return
	}
	if minLen, ok := s["minLength"].(float64); ok && float64(len(str)) < minLen {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("string length %d is less than minimum %d", len(str), int(minLen))})
	}
	if maxLen, ok := s["maxLength"].(float64); ok && float64(len(str)) > maxLen {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("string length %d exceeds maximum %d", len(str), int(maxLen))})
	}
	if enum, ok := s["enum"].([]any); ok {
		for _, e := range enum {
			if e == str {
				return
			}
		}
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("value %q is not in enum %v", str, enum)})
	}
}

func validateNumber(s JSON, schemaType string, value any, path string, errs *[]ValidationError) {
	num, ok := value.(float64)
	if !ok {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected number, got %T", value)})
		return
	}
	if schemaType == "integer" && num != float64(int64(num)) {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected integer, got %v", num)})
	}
	if minimum, ok := s["minimum"].(float64); ok && num < minimum {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("value %v is less than minimum %v", num, minimum)})
	}
	if maximum, ok := s["maximum"].(float64); ok && num > maximum {
		*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("value %v exceeds maximum %v", num, maximum)})
	}
}
