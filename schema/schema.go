package schema

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/invopop/jsonschema"
)

// JSON is a standard JSON Schema document represented as a map.
// Using map[string]any keeps the schema format-agnostic and easily
// serializable to any wire format (OpenAI, Anthropic, MCP, etc.).
type JSON = map[string]any

// Generate creates a JSON Schema from a Go type using struct tags.
// The type parameter T should be a struct with json and optional
// jsonschema tags.
//
//	schema.Generate[SearchInput]()
//	schema.Generate[SearchInput](schema.WithTitle("Search"), schema.WithDescription("..."))
func Generate[T any](opts ...Option) JSON {
	cfg := applyOptions(opts)
	r := newReflector(cfg)
	s := r.Reflect(new(T))
	applyOverrides(s, cfg)
	return toJSON(s)
}

// From creates a JSON Schema from a reflect.Type. Use this when the
// type is not known at compile time.
func From(t reflect.Type, opts ...Option) JSON {
	cfg := applyOptions(opts)
	r := newReflector(cfg)
	s := r.ReflectFromType(t)
	applyOverrides(s, cfg)
	return toJSON(s)
}

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

// Validate checks a value against a JSON Schema and returns validation results.
// The schema should be a JSON Schema document (as returned by Generate or From).
// The value can be any Go value that is JSON-serializable.
//
//	result := schema.Validate(mySchema, input)
//	if !result.Valid {
//	    for _, err := range result.Errors {
//	        log.Printf("validation error at %s: %s", err.Path, err.Message)
//	    }
//	}
func Validate(s JSON, value any) ValidationResult {
	if s == nil {
		return ValidationResult{Valid: true}
	}

	if value == nil {
		return ValidationResult{
			Valid:  false,
			Errors: []ValidationError{{Message: "value is nil"}},
		}
	}

	// Convert value to map[string]any for inspection
	var data any
	switch v := value.(type) {
	case json.RawMessage:
		if err := json.Unmarshal(v, &data); err != nil {
			return ValidationResult{
				Valid:  false,
				Errors: []ValidationError{{Message: fmt.Sprintf("invalid JSON: %v", err)}},
			}
		}
	case []byte:
		if err := json.Unmarshal(v, &data); err != nil {
			return ValidationResult{
				Valid:  false,
				Errors: []ValidationError{{Message: fmt.Sprintf("invalid JSON: %v", err)}},
			}
		}
	default:
		// Marshal then unmarshal to get a generic representation
		b, err := json.Marshal(value)
		if err != nil {
			return ValidationResult{
				Valid:  false,
				Errors: []ValidationError{{Message: fmt.Sprintf("cannot serialize value: %v", err)}},
			}
		}
		if err := json.Unmarshal(b, &data); err != nil {
			return ValidationResult{
				Valid:  false,
				Errors: []ValidationError{{Message: fmt.Sprintf("cannot deserialize value: %v", err)}},
			}
		}
	}

	var errs []ValidationError
	validateValue(s, data, "", &errs)

	return ValidationResult{
		Valid:  len(errs) == 0,
		Errors: errs,
	}
}

// validateValue recursively validates a value against a schema.
func validateValue(s JSON, value any, path string, errs *[]ValidationError) {
	schemaType, _ := s["type"].(string)

	// Type check
	switch schemaType {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			if value == nil {
				obj = map[string]any{}
			} else {
				*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected object, got %T", value)})
				return
			}
		}

		// Check required fields
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

		// Validate properties
		if properties, ok := s["properties"].(map[string]any); ok {
			for key, propSchema := range properties {
				if ps, ok := propSchema.(map[string]any); ok {
					if val, exists := obj[key]; exists {
						fieldPath := path + "/" + key
						validateValue(ps, val, fieldPath, errs)
					}
				}
			}
		}

	case "array":
		arr, ok := value.([]any)
		if !ok {
			*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected array, got %T", value)})
			return
		}
		// Validate items
		if items, ok := s["items"].(map[string]any); ok {
			for i, item := range arr {
				itemPath := fmt.Sprintf("%s/%d", path, i)
				validateValue(items, item, itemPath, errs)
			}
		}
		// Validate minItems/maxItems
		if minItems, ok := s["minItems"].(float64); ok {
			if float64(len(arr)) < minItems {
				*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("array has %d items, minimum is %d", len(arr), int(minItems))})
			}
		}
		if maxItems, ok := s["maxItems"].(float64); ok {
			if float64(len(arr)) > maxItems {
				*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("array has %d items, maximum is %d", len(arr), int(maxItems))})
			}
		}

	case "string":
		str, ok := value.(string)
		if !ok {
			*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected string, got %T", value)})
			return
		}
		if minLen, ok := s["minLength"].(float64); ok {
			if float64(len(str)) < minLen {
				*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("string length %d is less than minimum %d", len(str), int(minLen))})
			}
		}
		if maxLen, ok := s["maxLength"].(float64); ok {
			if float64(len(str)) > maxLen {
				*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("string length %d exceeds maximum %d", len(str), int(maxLen))})
			}
		}
		// Validate enum
		if enum, ok := s["enum"].([]any); ok {
			found := false
			for _, e := range enum {
				if e == str {
					found = true
					break
				}
			}
			if !found {
				*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("value %q is not in enum %v", str, enum)})
			}
		}

	case "number", "integer":
		num, ok := value.(float64)
		if !ok {
			*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected number, got %T", value)})
			return
		}
		if schemaType == "integer" && num != float64(int64(num)) {
			*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected integer, got %v", num)})
		}
		if minimum, ok := s["minimum"].(float64); ok {
			if num < minimum {
				*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("value %v is less than minimum %v", num, minimum)})
			}
		}
		if maximum, ok := s["maximum"].(float64); ok {
			if num > maximum {
				*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("value %v exceeds maximum %v", num, maximum)})
			}
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			*errs = append(*errs, ValidationError{Path: path, Message: fmt.Sprintf("expected boolean, got %T", value)})
		}
	}
}

// newReflector creates a configured jsonschema.Reflector for tool-oriented
// schema generation.
func newReflector(cfg *config) *jsonschema.Reflector {
	return &jsonschema.Reflector{
		Anonymous:                  true,
		DoNotReference:             cfg.inline,
		AllowAdditionalProperties:  cfg.allowAdditional,
		RequiredFromJSONSchemaTags: true,
	}
}

// applyOverrides sets title and description on the root schema if provided.
func applyOverrides(s *jsonschema.Schema, cfg *config) {
	if cfg.title != "" {
		s.Title = cfg.title
	}
	if cfg.description != "" {
		s.Description = cfg.description
	}
}

// toJSON converts a *jsonschema.Schema to a plain map[string]any.
func toJSON(s *jsonschema.Schema) JSON {
	b, err := json.Marshal(s)
	if err != nil {
		return JSON{"type": "object"}
	}
	var m JSON
	if err := json.Unmarshal(b, &m); err != nil {
		return JSON{"type": "object"}
	}
	return m
}
