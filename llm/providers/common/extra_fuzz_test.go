package common

import (
	"encoding/json"
	"testing"
)

func FuzzMergeExtra(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("{}"))
	f.Add([]byte(`{"a":1}`))
	f.Add([]byte(`{"nested":{"x":[1,2]}}`))
	f.Add([]byte("[1,2,3]"))
	f.Add([]byte(`{"a":`))
	f.Fuzz(func(t *testing.T, extra []byte) {
		body := map[string]any{"model": "m"}
		// Must never panic; on error the body must be usable and re-marshalable.
		if err := MergeExtra(body, json.RawMessage(extra)); err != nil {
			return
		}
		if _, err := json.Marshal(body); err != nil {
			t.Fatalf("merged body not marshalable: %v", err)
		}
	})
}
