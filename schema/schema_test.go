package schema_test

import (
	"reflect"
	"testing"

	"github.com/kbukum/gokit/schema"
)

type SimpleInput struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type AnnotatedInput struct {
	Query    string `json:"query"    jsonschema:"required,description=Search query text"`
	Platform string `json:"platform" jsonschema:"enum=youtube,enum=tiktok,enum=instagram"`
	Limit    int    `json:"limit"    jsonschema:"minimum=1,maximum=100"`
}

type NestedInput struct {
	Filter FilterConfig `json:"filter"`
	Page   int          `json:"page"`
}

type FilterConfig struct {
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
}

func TestGenerate_Simple(t *testing.T) {
	s := schema.Generate[SimpleInput]()
	if s == nil {
		t.Fatal("expected non-nil schema")
	}
	if s["type"] != "object" {
		t.Errorf("expected type=object, got %v", s["type"])
	}
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, exists := props["name"]; !exists {
		t.Error("expected 'name' property")
	}
	if _, exists := props["count"]; !exists {
		t.Error("expected 'count' property")
	}
}

func TestGenerate_Annotated(t *testing.T) {
	s := schema.Generate[AnnotatedInput]()
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}

	// Check query has description
	query, ok := props["query"].(map[string]any)
	if !ok {
		t.Fatal("expected query property map")
	}
	if query["description"] != "Search query text" {
		t.Errorf("expected query description, got %v", query["description"])
	}

	// Check platform has enum
	platform, ok := props["platform"].(map[string]any)
	if !ok {
		t.Fatal("expected platform property map")
	}
	enumVals, ok := platform["enum"].([]any)
	if !ok {
		t.Fatal("expected platform enum array")
	}
	if len(enumVals) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(enumVals))
	}

	// Check required includes query
	required, ok := s["required"].([]any)
	if !ok {
		t.Fatal("expected required array")
	}
	found := false
	for _, r := range required {
		if r == "query" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'query' in required fields")
	}
}

func TestGenerate_WithOptions(t *testing.T) {
	s := schema.Generate[SimpleInput](
		schema.WithTitle("My Schema"),
		schema.WithDescription("A test schema"),
	)
	if s["title"] != "My Schema" {
		t.Errorf("expected title, got %v", s["title"])
	}
	if s["description"] != "A test schema" {
		t.Errorf("expected description, got %v", s["description"])
	}
}

func TestGenerate_Nested(t *testing.T) {
	s := schema.Generate[NestedInput]()
	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	filter, ok := props["filter"].(map[string]any)
	if !ok {
		t.Fatal("expected filter property map")
	}
	// Inlined nested type should have properties directly
	filterProps, ok := filter["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected filter.properties map (inlined)")
	}
	if _, exists := filterProps["category"]; !exists {
		t.Error("expected 'category' in filter properties")
	}
}

func TestFrom_ReflectType(t *testing.T) {
	s := schema.From(reflect.TypeOf(SimpleInput{}))
	if s == nil {
		t.Fatal("expected non-nil schema")
	}
	if s["type"] != "object" {
		t.Errorf("expected type=object, got %v", s["type"])
	}
}

func TestFrom_WithOptions(t *testing.T) {
	s := schema.From(
		reflect.TypeOf(SimpleInput{}),
		schema.WithTitle("Reflected"),
	)
	if s["title"] != "Reflected" {
		t.Errorf("expected title, got %v", s["title"])
	}
}

// --- Validate tests ---

func TestValidate_ValidObject(t *testing.T) {
	s := schema.Generate[AnnotatedInput]()
	vr := schema.Validate(s, map[string]any{
		"query":    "hello",
		"platform": "youtube",
		"limit":    10,
	})
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}
}

func TestValidate_MissingRequired(t *testing.T) {
	s := schema.Generate[AnnotatedInput]()
	vr := schema.Validate(s, map[string]any{
		"platform": "youtube",
	})
	if vr.Valid {
		t.Error("expected invalid for missing required field 'query'")
	}
	found := false
	for _, e := range vr.Errors {
		if e.Path == "query" {
			found = true
		}
	}
	if !found {
		t.Error("expected validation error for 'query' path")
	}
}

func TestValidate_WrongType(t *testing.T) {
	s := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
	}
	vr := schema.Validate(s, map[string]any{
		"count": "not-a-number",
	})
	if vr.Valid {
		t.Error("expected invalid for wrong type")
	}
}

func TestValidate_EnumValue(t *testing.T) {
	s := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"color": map[string]any{
				"type": "string",
				"enum": []any{"red", "green", "blue"},
			},
		},
	}

	vr := schema.Validate(s, map[string]any{"color": "red"})
	if !vr.Valid {
		t.Errorf("expected valid enum, got errors: %v", vr.Errors)
	}

	vr = schema.Validate(s, map[string]any{"color": "purple"})
	if vr.Valid {
		t.Error("expected invalid for enum value 'purple'")
	}
}

func TestValidate_StringLength(t *testing.T) {
	s := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":      "string",
				"minLength": float64(2),
				"maxLength": float64(10),
			},
		},
	}

	vr := schema.Validate(s, map[string]any{"name": "ok"})
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}

	vr = schema.Validate(s, map[string]any{"name": "x"})
	if vr.Valid {
		t.Error("expected invalid for too short string")
	}

	vr = schema.Validate(s, map[string]any{"name": "this-is-way-too-long"})
	if vr.Valid {
		t.Error("expected invalid for too long string")
	}
}

func TestValidate_NumberRange(t *testing.T) {
	s := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"age": map[string]any{
				"type":    "number",
				"minimum": float64(0),
				"maximum": float64(150),
			},
		},
	}

	vr := schema.Validate(s, map[string]any{"age": float64(25)})
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}

	vr = schema.Validate(s, map[string]any{"age": float64(-1)})
	if vr.Valid {
		t.Error("expected invalid for age < 0")
	}

	vr = schema.Validate(s, map[string]any{"age": float64(200)})
	if vr.Valid {
		t.Error("expected invalid for age > 150")
	}
}

func TestValidate_ArrayItems(t *testing.T) {
	s := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tags": map[string]any{
				"type":     "array",
				"items":    map[string]any{"type": "string"},
				"minItems": float64(1),
				"maxItems": float64(5),
			},
		},
	}

	vr := schema.Validate(s, map[string]any{"tags": []any{"go", "rust"}})
	if !vr.Valid {
		t.Errorf("expected valid, got errors: %v", vr.Errors)
	}

	vr = schema.Validate(s, map[string]any{"tags": []any{}})
	if vr.Valid {
		t.Error("expected invalid for empty array with minItems=1")
	}
}

func TestValidate_NilValue(t *testing.T) {
	s := map[string]any{"type": "object"}
	vr := schema.Validate(s, nil)
	if vr.Valid {
		t.Error("expected invalid for nil value")
	}
}

func TestValidate_NestedObject(t *testing.T) {
	s := schema.Generate[NestedInput]()
	vr := schema.Validate(s, map[string]any{
		"filter": map[string]any{
			"category": "tech",
			"tags":     []any{"go"},
		},
		"page": float64(1),
	})
	if !vr.Valid {
		t.Errorf("expected valid nested object, got errors: %v", vr.Errors)
	}
}
