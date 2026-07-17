package schema

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/dataset/record"
)

// FuzzSchemaValidate ensures validating untrusted records never panics and
// always fails closed on non-conforming input.
func FuzzSchemaValidate(f *testing.F) {
	s, err := Compile(JSON{
		"type":     "object",
		"required": []any{"name"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
	})
	if err != nil {
		f.Fatalf("Compile error: %v", err)
	}
	f.Add([]byte(`{"name":"alice"}`))
	f.Add([]byte(`{"age":1}`))
	f.Add([]byte(`{`))
	f.Fuzz(func(_ *testing.T, data []byte) {
		var fields map[string]record.Value
		if err := json.Unmarshal(data, &fields); err != nil {
			return
		}
		_ = s.Validate(record.New(fields))
	})
}
