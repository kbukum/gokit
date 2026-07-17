package schema

import (
	"errors"
	"testing"

	"github.com/kbukum/gokit/dataset/record"
	apperrors "github.com/kbukum/gokit/errors"
)

func personSchema(t *testing.T) *Schema {
	t.Helper()
	s, err := Compile(JSON{
		"type":     "object",
		"required": []any{"name"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "integer", "minimum": float64(0)},
		},
	})
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}
	return s
}

func TestSchemaValidateAccepts(t *testing.T) {
	t.Parallel()
	s := personSchema(t)
	if err := s.Validate(record.New(map[string]record.Value{"name": "alice", "age": float64(30)})); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
}

func TestSchemaValidateFailsClosed(t *testing.T) {
	t.Parallel()
	s := personSchema(t)
	err := s.Validate(record.New(map[string]record.Value{"age": float64(30)}))
	if err == nil {
		t.Fatal("record missing required field should fail")
	}
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
}

func TestSchemaValidateWrongType(t *testing.T) {
	t.Parallel()
	s := personSchema(t)
	if err := s.Validate(record.New(map[string]record.Value{"name": float64(1)})); err == nil {
		t.Fatal("wrong-typed field should fail")
	}
}

func TestNilSchemaAcceptsAll(t *testing.T) {
	t.Parallel()
	var s *Schema
	if err := s.Validate(record.New(map[string]record.Value{"anything": true})); err != nil {
		t.Fatalf("nil schema should accept any record, got %v", err)
	}
}
