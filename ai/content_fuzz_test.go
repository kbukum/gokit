package ai

import (
	"encoding/json"
	"testing"
)

func FuzzNormalizeToolInput(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("null"))
	f.Add([]byte("  {}  "))
	f.Add([]byte(`{"a":1}`))
	f.Add([]byte("[1,2,3]"))
	f.Add([]byte("not json"))
	f.Fuzz(func(t *testing.T, in []byte) {
		out := NormalizeToolInput(json.RawMessage(in))
		// Postcondition: output is never empty and never the JSON null literal.
		if len(out) == 0 {
			t.Fatalf("normalized output is empty for input %q", in)
		}
		if string(out) == "null" {
			t.Fatalf("normalized output is null for input %q", in)
		}
	})
}
