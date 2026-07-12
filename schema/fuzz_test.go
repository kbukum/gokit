package schema

import (
	"encoding/json"
	"testing"
)

// FuzzValidate ensures the validator never panics on arbitrary schema and value
// JSON, including malformed and adversarially nested input.
func FuzzValidate(f *testing.F) {
	f.Add(`{"type":"object","properties":{"a":{"type":"string"}}}`, `{"a":"x"}`)
	f.Add(`{"type":"array","items":{"type":"integer"}}`, `[1,2,3]`)
	f.Add(`{"type":"string","enum":["a","b"]}`, `"c"`)
	f.Add(`{}`, `null`)
	f.Add(`not json`, `also not json`)

	f.Fuzz(func(t *testing.T, schemaJSON, valueJSON string) {
		var s JSON
		if err := json.Unmarshal([]byte(schemaJSON), &s); err != nil {
			return
		}
		c, err := Compile(s)
		if err != nil {
			return
		}
		// Must not panic regardless of value contents.
		_ = c.Validate(json.RawMessage(valueJSON))
	})
}
