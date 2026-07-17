package record

import (
	"maps"
	"slices"
)

// Value is a single decoded-JSON field value inside a [Record].
//
// The any element type is a deliberate, documented exception to the no-any
// rule: a record field carries heterogeneous JSON leaf data (string, float64,
// bool, nil, and nested map/slice) whose leaves cannot be given a closed type.
// It matches [github.com/kbukum/gokit/codec] Value and schema.JSON.
type Value = any

// Record is one dataset row: an ordered set of named JSON field values. Keys
// are compared in sorted order so serialization is deterministic.
type Record struct {
	fields map[string]Value
}

// New returns a Record over a copy of fields, so later mutation of the caller's
// map does not affect the record.
func New(fields map[string]Value) Record {
	return Record{fields: maps.Clone(fields)}
}

// Get returns the value for name and whether it was present.
func (r Record) Get(name string) (Value, bool) {
	v, ok := r.fields[name]
	return v, ok
}

// Len reports the number of fields in the record.
func (r Record) Len() int { return len(r.fields) }

// Keys returns the field names in sorted order.
func (r Record) Keys() []string {
	keys := make([]string, 0, len(r.fields))
	for k := range r.fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Fields returns a copy of the record's fields; mutating it does not affect the
// record.
func (r Record) Fields() map[string]Value {
	return maps.Clone(r.fields)
}

// Select returns a new record containing only the named columns that are
// present, preserving their values.
func (r Record) Select(columns []string) Record {
	out := make(map[string]Value, len(columns))
	for _, c := range columns {
		if v, ok := r.fields[c]; ok {
			out[c] = v
		}
	}
	return Record{fields: out}
}

// ToJSON returns the record as a JSON-serializable map (a copy of its fields).
func (r Record) ToJSON() map[string]Value {
	return maps.Clone(r.fields)
}
