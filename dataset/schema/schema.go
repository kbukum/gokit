package schema

import (
	"strings"

	"github.com/kbukum/gokit/dataset/record"
	apperrors "github.com/kbukum/gokit/errors"
	gokitschema "github.com/kbukum/gokit/schema"
)

// JSON is a JSON Schema document, re-exported from the canonical schema owner
// so callers need not import both packages.
type JSON = gokitschema.JSON

// Schema validates records against a compiled JSON Schema. It fails closed: any structural
// or validation error rejects the record.
type Schema struct {
	compiled *gokitschema.CompiledSchema
}

// Compile compiles a JSON Schema document for repeated record validation.
// A nil document compiles to a schema that accepts any record.
func Compile(doc JSON) (*Schema, error) {
	compiled, err := gokitschema.Compile(doc)
	if err != nil {
		return nil, apperrors.InvalidInput("schema", "failed to compile dataset schema").WithCause(err)
	}
	return &Schema{compiled: compiled}, nil
}

// Validate checks a record against the schema,
// returning a typed InvalidInput error aggregating every failure. A nil Schema accepts any record.
func (s *Schema) Validate(rec record.Record) error {
	if s == nil || s.compiled == nil {
		return nil
	}
	result := s.compiled.Validate(rec.ToJSON())
	if result.Valid {
		return nil
	}
	msgs := make([]string, 0, len(result.Errors))
	for _, e := range result.Errors {
		msgs = append(msgs, e.Error())
	}
	return apperrors.InvalidInput("record", strings.Join(msgs, "; "))
}
