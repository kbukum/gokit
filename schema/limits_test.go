package schema

import (
	"errors"
	"strings"
	"testing"
)

func TestDefaultLimits(t *testing.T) {
	t.Parallel()
	l := DefaultLimits()
	if l.MaxDepth != 128 || l.MaxNodes != 100_000 {
		t.Errorf("unexpected default limits: %+v", l)
	}
	if l.MaxStringBytes != 1<<20 || l.MaxKeyBytes != 16_384 || l.MaxTotalStringBytes != 1<<24 {
		t.Errorf("unexpected default byte limits: %+v", l)
	}
}

func TestCompileRejectsDeepSchema(t *testing.T) {
	t.Parallel()
	limits := ValidationLimits{MaxDepth: 3, MaxNodes: 1000, MaxStringBytes: 1000, MaxKeyBytes: 1000, MaxTotalStringBytes: 10000}
	// Build a schema nested deeper than MaxDepth.
	deep := JSON{"type": "object"}
	cur := deep
	for i := 0; i < 5; i++ {
		next := map[string]any{"type": "object"}
		cur["properties"] = map[string]any{"child": next}
		cur = next
	}
	_, err := CompileWithLimits(deep, limits)
	if err == nil {
		t.Fatal("expected depth limit error, got nil")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T", err)
	}
	if le.Subject != "schema" || !strings.Contains(le.Message, "depth") {
		t.Errorf("unexpected LimitError: %+v", le)
	}
}

func TestCompileRejectsTooManyNodes(t *testing.T) {
	t.Parallel()
	limits := ValidationLimits{MaxDepth: 100, MaxNodes: 3, MaxStringBytes: 1000, MaxKeyBytes: 1000, MaxTotalStringBytes: 10000}
	s := JSON{"type": "object", "properties": map[string]any{
		"a": map[string]any{"type": "string"},
		"b": map[string]any{"type": "string"},
	}}
	_, err := CompileWithLimits(s, limits)
	if err == nil {
		t.Fatal("expected node-count limit error")
	}
	if !strings.Contains(err.Error(), "node count") {
		t.Errorf("expected node count message, got %q", err.Error())
	}
}

func TestCompileRejectsLongKey(t *testing.T) {
	t.Parallel()
	limits := ValidationLimits{MaxDepth: 100, MaxNodes: 1000, MaxStringBytes: 1000, MaxKeyBytes: 4, MaxTotalStringBytes: 10000}
	s := JSON{"type": "object", "properties": map[string]any{
		"averylongkey": map[string]any{"type": "string"},
	}}
	_, err := CompileWithLimits(s, limits)
	if err == nil || !strings.Contains(err.Error(), "object key") {
		t.Fatalf("expected object key limit error, got %v", err)
	}
}

func TestValidateRejectsOversizedValue(t *testing.T) {
	t.Parallel()
	limits := ValidationLimits{MaxDepth: 100, MaxNodes: 1000, MaxStringBytes: 8, MaxKeyBytes: 1000, MaxTotalStringBytes: 10000}
	s := JSON{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}
	c, err := CompileWithLimits(s, limits)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := c.Validate(map[string]any{"name": "this string is definitely longer than eight bytes"})
	if res.Valid {
		t.Fatal("expected value to be rejected by string-byte limit")
	}
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0].Message, "string value exceeds") {
		t.Errorf("unexpected errors: %+v", res.Errors)
	}
}

func TestNilLimitsCheckPasses(t *testing.T) {
	t.Parallel()
	if err := DefaultLimits().check("schema", nil); err != nil {
		t.Errorf("nil document should pass limits, got %v", err)
	}
}
