package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/contracttest/golden"
	"github.com/kbukum/gokit/schema"
)

// The ValidationResult wire shape is a cross-kit contract shared with rskit:
// a bare {"valid":true} on success and {"valid":false,"errors":[{"path",
// "message"}]} on failure, with RFC 6901 JSON-pointer paths. These goldens pin
// the same examples as rskit's validation-result.{valid,invalid}.json fixtures.

func TestValidationResultValidGoldenJSON(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(schema.ValidationResult{Valid: true})
	if err != nil {
		t.Fatalf("marshal ValidationResult: %v", err)
	}
	golden.AssertJSON(t, data, `{"valid":true}`)
}

func TestValidationResultInvalidGoldenJSON(t *testing.T) {
	t.Parallel()

	// Drive the invalid result through the real validator so the golden pins
	// the production wire shape and JSON-pointer path, not a hand-built message
	// the validator can never emit.
	result := schema.Validate(
		schema.JSON{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "integer"}},
		},
		map[string]any{"query": "x"},
	)
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal ValidationResult: %v", err)
	}
	golden.AssertJSON(t, data, `{
		"valid": false,
		"errors": [
			{"path": "/query", "message": "got string, want integer"}
		]
	}`)
}

func TestValidationResultNestedPointerGoldenJSON(t *testing.T) {
	t.Parallel()

	// A missing nested property must report the child pointer (/user/id), and a
	// property name containing '/' must be JSON-pointer escaped (~1), proving the
	// path is derived from typed error data rather than parsed message text.
	result := schema.Validate(
		schema.JSON{
			"type": "object",
			"properties": map[string]any{
				"user": map[string]any{
					"type":                 "object",
					"required":             []any{"id", "a/b"},
					"additionalProperties": false,
					"properties": map[string]any{
						"id":  map[string]any{"type": "string"},
						"a/b": map[string]any{"type": "string"},
					},
				},
			},
		},
		map[string]any{"user": map[string]any{}},
	)
	if result.Valid {
		t.Fatalf("expected invalid result")
	}
	paths := make(map[string]bool, len(result.Errors))
	for _, e := range result.Errors {
		paths[e.Path] = true
	}
	for _, want := range []string{"/user/id", "/user/a~1b"} {
		if !paths[want] {
			t.Fatalf("missing expected pointer %q in %+v", want, result.Errors)
		}
	}
}
