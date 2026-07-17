package schema

import (
	"testing"

	"github.com/kbukum/gokit/dataset/record"
	apperrors "github.com/kbukum/gokit/errors"
)

func TestSchemaValidatorAdapter(t *testing.T) {
	t.Parallel()
	s, err := Compile(JSON{
		"type":     "object",
		"required": []any{"name"},
		"properties": JSON{
			"name": JSON{"type": "string"},
		},
	})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	v := s.Validator()

	if err := v.Validate(record.New(map[string]record.Value{"name": "ok"})); err != nil {
		t.Fatalf("Validate(valid) = %v; want nil", err)
	}

	err = v.Validate(record.New(map[string]record.Value{"other": 1}))
	if err == nil {
		t.Fatal("Validate(invalid) = nil; want typed error")
	}
	appErr, ok := apperrors.AsAppError(err)
	if !ok || appErr.Code != apperrors.ErrCodeInvalidInput {
		t.Fatalf("Validate(invalid) = %v; want InvalidInput AppError", err)
	}
}

func TestNilSchemaValidatorAcceptsAll(t *testing.T) {
	t.Parallel()
	var s *Schema
	if err := s.Validator().Validate(record.New(map[string]record.Value{"x": 1})); err != nil {
		t.Fatalf("nil-schema Validate = %v; want nil", err)
	}
}
