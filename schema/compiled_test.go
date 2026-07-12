package schema

import "testing"

func TestCompileReuse(t *testing.T) {
	t.Parallel()
	s := Generate[struct {
		Name string `json:"name" jsonschema:"required"`
		Age  int    `json:"age"`
	}]()
	c, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if res := c.Validate(map[string]any{"name": "ada", "age": 30}); !res.Valid {
		t.Errorf("expected valid, got %+v", res.Errors)
	}
	if res := c.Validate(map[string]any{"age": 30}); res.Valid {
		t.Error("expected missing-required failure on reuse")
	}
}

func TestCompileNilSchemaAcceptsAny(t *testing.T) {
	t.Parallel()
	c, err := Compile(nil)
	if err != nil {
		t.Fatalf("compile nil: %v", err)
	}
	if res := c.Validate(map[string]any{"anything": true}); !res.Valid {
		t.Errorf("nil schema should accept any value, got %+v", res.Errors)
	}
}

func TestCompiledValidateNilValue(t *testing.T) {
	t.Parallel()
	c, err := Compile(JSON{"type": "object"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res := c.Validate(nil); res.Valid {
		t.Error("nil value should be invalid against a non-nil schema")
	}
}

func TestCompiledValidateRawMessage(t *testing.T) {
	t.Parallel()
	s := JSON{"type": "object", "required": []any{"name"}, "properties": map[string]any{
		"name": map[string]any{"type": "string"},
	}}
	c, err := Compile(s)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if res := c.Validate([]byte(`{"name":"ok"}`)); !res.Valid {
		t.Errorf("expected valid raw JSON, got %+v", res.Errors)
	}
	if res := c.Validate([]byte(`{"name":123}`)); res.Valid {
		t.Error("expected type error for numeric name")
	}
	if res := c.Validate([]byte(`{not json`)); res.Valid {
		t.Error("expected invalid JSON to fail")
	}
}
