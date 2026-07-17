package schema

import (
	"github.com/kbukum/gokit/dataset/record"
	"github.com/kbukum/gokit/dataset/stage"
)

// Validator adapts a [Schema] into a [stage.Validator] over records, so the
// generic collector can enforce a JSON Schema without depending on this
// package's concrete type. A nil Schema yields a validator that accepts every
// record, matching [Schema.Validate].
func (s *Schema) Validator() stage.Validator[record.Record] {
	return stage.ValidatorFunc[record.Record](s.Validate)
}
