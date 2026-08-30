package schema

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
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

// CompiledSchema is a JSON Schema document that has been checked against structural limits
// and is ready to validate values repeatedly without re-inspecting the schema itself.
type CompiledSchema struct {
	schema   JSON
	limits   ValidationLimits
	compiled *jsonschema.Schema
}

// Compile validates a schema document against the default structural limits
// and returns a reusable CompiledSchema.
// A nil schema compiles to a validator that accepts any value.
func Compile(s JSON) (*CompiledSchema, error) {
	return CompileWithLimits(s, DefaultLimits())
}

// CompileWithLimits is like Compile
// but applies caller-supplied structural limits to the schema document.
func CompileWithLimits(s JSON, limits ValidationLimits) (*CompiledSchema, error) {
	if s != nil {
		if err := limits.check("schema", s); err != nil {
			return nil, err
		}
		compiled, err := compileJSONSchema(s)
		if err != nil {
			return nil, err
		}
		return &CompiledSchema{schema: s, limits: limits, compiled: compiled}, nil
	}
	return &CompiledSchema{schema: s, limits: limits}, nil
}

// Validate checks a JSON-serializable value against the compiled schema,
// enforcing the compiled structural limits on the value before inspection.
//
// The value parameter is a documented opaque-value exception to the no-any rule:
// it accepts any JSON-serializable Go value (including json.RawMessage or a []byte JSON payload).
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

	if err := c.compiled.Validate(data); err != nil {
		return validationResultFromError(err)
	}
	return ValidationResult{Valid: true}
}

func compileJSONSchema(s JSON) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(noExternalLoader{})
	if err := compiler.AddResource("gokit://schema.json", s); err != nil {
		return nil, err
	}
	return compiler.Compile("gokit://schema.json")
}

// noExternalLoader refuses every external schema reference. The compiler only
// consults a URLLoader for a $ref/$schema URL that was not explicitly added as
// a resource, so untrusted schemas cannot read local files (file://) or reach
// out over the network; the added root and the built-in draft metaschemas are
// resolved without it.
type noExternalLoader struct{}

func (noExternalLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("schema: external reference %q is not permitted", url)
}

func validationResultFromError(err error) ValidationResult {
	var validationErr *jsonschema.ValidationError
	if !stderrors.As(err, &validationErr) {
		return invalidResult(err.Error())
	}

	var errs []ValidationError
	collectOutputErrors(validationErr.DetailedOutput(), &errs)
	return ValidationResult{Valid: len(errs) == 0, Errors: errs}
}

func collectOutputErrors(unit *jsonschema.OutputUnit, errs *[]ValidationError) {
	if unit == nil {
		return
	}
	for i := range unit.Errors {
		collectOutputErrors(&unit.Errors[i], errs)
	}
	if unit.Error == nil {
		return
	}
	message := unit.Error.String()
	// Prefer the typed error kind over parsing the localized message so nested
	// required/additionalProperties failures report the exact child pointer,
	// independent of message wording, escaping, or locale.
	switch k := unit.Error.Kind.(type) {
	case *kind.Required:
		for _, name := range k.Missing {
			*errs = append(*errs, ValidationError{
				Path:    childPointer(unit.InstanceLocation, name),
				Message: message,
			})
		}
	case *kind.AdditionalProperties:
		for _, name := range k.Properties {
			*errs = append(*errs, ValidationError{
				Path:    childPointer(unit.InstanceLocation, name),
				Message: message,
			})
		}
	default:
		*errs = append(*errs, ValidationError{
			Path:    unit.InstanceLocation,
			Message: message,
		})
	}
}

// childPointer appends a property name to an RFC 6901 JSON pointer, escaping the
// token so names containing '/' or '~' produce a valid pointer.
func childPointer(parent, name string) string {
	return parent + "/" + escapeJSONPointerToken(name)
}

func escapeJSONPointerToken(token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	return strings.ReplaceAll(token, "/", "~1")
}

// Validate checks a value against a JSON Schema and returns validation results.
// It compiles the schema with default limits on each call;
// prefer Compile plus CompiledSchema.Validate when validating many values against one schema.
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
