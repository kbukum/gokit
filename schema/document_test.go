package schema

import "testing"

type genInput struct {
	Query string `json:"query" jsonschema:"required"`
	Limit int    `json:"limit"`
}

func TestSchemaDocumentRoundTrip(t *testing.T) {
	t.Parallel()

	raw := JSON{"type": "object", "properties": JSON{"a": JSON{"type": "string"}}}
	doc, err := NewSchemaDocument(raw)
	if err != nil {
		t.Fatalf("NewSchemaDocument: %v", err)
	}
	if doc.AsJSON()["type"] != "object" {
		t.Errorf("AsJSON type = %v", doc.AsJSON()["type"])
	}
	out := doc.IntoJSON()
	if out["type"] != "object" {
		t.Errorf("IntoJSON type = %v", out["type"])
	}
	if doc.AsJSON() != nil {
		t.Error("expected document detached after IntoJSON")
	}
}

func TestNewSchemaDocumentEnforcesLimits(t *testing.T) {
	t.Parallel()

	limits := DefaultLimits()
	limits.MaxNodes = 1
	_, err := NewSchemaDocumentWithLimits(JSON{"type": "object", "properties": JSON{"a": JSON{}}}, limits)
	if err == nil {
		t.Fatal("expected limit violation")
	}
}

func TestGenerateDocument(t *testing.T) {
	t.Parallel()

	doc, err := GenerateDocument[genInput](WithTitle("Gen"))
	if err != nil {
		t.Fatalf("GenerateDocument: %v", err)
	}
	if doc.AsJSON()["title"] != "Gen" {
		t.Errorf("title = %v, want Gen", doc.AsJSON()["title"])
	}
	if doc.AsJSON()["type"] != "object" {
		t.Errorf("type = %v, want object", doc.AsJSON()["type"])
	}
}

func TestValidateWithOptions(t *testing.T) {
	t.Parallel()

	s := JSON{
		"type":     "object",
		"required": []any{"query"},
		"properties": JSON{
			"query": JSON{"type": "string"},
		},
	}

	res, err := ValidateWithOptions(s, map[string]any{"query": "hello"}, DefaultOptions())
	if err != nil {
		t.Fatalf("ValidateWithOptions: %v", err)
	}
	if !res.Valid {
		t.Errorf("expected valid, got %+v", res)
	}

	res, err = ValidateWithOptions(s, map[string]any{}, DefaultOptions())
	if err != nil {
		t.Fatalf("ValidateWithOptions: %v", err)
	}
	if res.Valid {
		t.Error("expected invalid for missing required field")
	}
}

func TestValidateWithOptionsValueLimitError(t *testing.T) {
	t.Parallel()

	opts := ValidationOptions{Limits: DefaultLimits()}
	opts.Limits.MaxNodes = 1
	s := JSON{"type": "object"}
	_, err := ValidateWithOptions(s, map[string]any{"a": 1, "b": 2}, opts)
	if err == nil {
		t.Fatal("expected value limit error")
	}
}

func TestValidateStructuredOutput(t *testing.T) {
	t.Parallel()

	s := JSON{"type": "object", "required": []any{"ok"}, "properties": JSON{"ok": JSON{"type": "boolean"}}}
	if res := ValidateStructuredOutput(s, map[string]any{"ok": true}); !res.Valid {
		t.Errorf("expected valid, got %+v", res)
	}
	if res := ValidateStructuredOutput(s, map[string]any{}); res.Valid {
		t.Error("expected invalid for missing field")
	}
}
