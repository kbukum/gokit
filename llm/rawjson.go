package llm

import (
	"encoding/json"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// RawJSON carries a raw JSON value verbatim through the llm API. It is the typed carrier for provider-specific request extensions ([CompletionRequest.Extra]): the bytes stay opaque so the public surface is free of any, and each provider dialect decodes them at its own wire boundary.
//
// RawJSON round-trips through both JSON and YAML, so a request may be authored in either format alongside the rest of [CompletionRequest]; a YAML value is normalized to its JSON encoding on decode.
type RawJSON []byte

// MarshalJSON returns the raw bytes unchanged, or "null" when empty. It fails closed when the bytes are not a valid JSON value, so a RawJSON constructed from arbitrary bytes cannot silently emit a malformed request payload.
func (m RawJSON) MarshalJSON() ([]byte, error) {
	if len(m) == 0 {
		return []byte("null"), nil
	}
	if !json.Valid(m) {
		return nil, fmt.Errorf("llm.RawJSON: invalid JSON value")
	}
	return m, nil
}

// UnmarshalJSON stores a copy of the raw JSON bytes without decoding them.
func (m *RawJSON) UnmarshalJSON(data []byte) error {
	if m == nil {
		return fmt.Errorf("llm.RawJSON: UnmarshalJSON on nil pointer")
	}
	*m = append((*m)[:0], data...)
	return nil
}

// UnmarshalYAML decodes a YAML node and stores its equivalent JSON encoding, so YAML-authored extensions are carried in the same opaque JSON form as JSON input.
func (m *RawJSON) UnmarshalYAML(value *yaml.Node) error {
	if m == nil {
		return fmt.Errorf("llm.RawJSON: UnmarshalYAML on nil pointer")
	}
	var decoded any
	if err := value.Decode(&decoded); err != nil {
		return fmt.Errorf("llm.RawJSON: decode yaml: %w", err)
	}
	if decoded == nil {
		*m = nil
		return nil
	}
	data, err := json.Marshal(decoded)
	if err != nil {
		return fmt.Errorf("llm.RawJSON: encode json: %w", err)
	}
	*m = data
	return nil
}

// MarshalYAML renders the raw JSON as its decoded value so it serializes as a native YAML node rather than an opaque byte string. Empty input serializes as an explicit YAML null.
func (m RawJSON) MarshalYAML() (any, error) {
	if len(m) == 0 {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	var decoded any
	if err := json.Unmarshal(m, &decoded); err != nil {
		return nil, fmt.Errorf("llm.RawJSON: decode json: %w", err)
	}
	return decoded, nil
}
