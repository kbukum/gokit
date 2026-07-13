package dialect

import (
	"bytes"
	"encoding/json"
)

// MergeExtra decodes a raw JSON object of provider-specific request extensions
// and copies its top-level members into body, which is the request map a
// dialect is assembling. Empty or JSON-null extras are a no-op.
//
// It returns an error when extra is present but is not a JSON object, so a
// malformed extension fails closed instead of silently corrupting the request.
func MergeExtra(body map[string]any, extra json.RawMessage) error {
	trimmed := bytes.TrimSpace(extra)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return err
	}
	for k, v := range fields {
		var decoded any
		if err := json.Unmarshal(v, &decoded); err != nil {
			return err
		}
		body[k] = decoded
	}
	return nil
}
