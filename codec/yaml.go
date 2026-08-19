package codec

import (
	yaml "go.yaml.in/yaml/v3"

	apperrors "github.com/kbukum/gokit/errors"
)

// YAMLCodec is the built-in YAML codec.
//
// It decodes YAML into the canonical [Value] tree and encodes a value tree back to YAML.
// Like the TOML codec, the top-level value must be a mapping (the config-shaped document
// contract), and a non-mapping top level surfaces as a typed error rather than a panic —
// on both decode and encode, so round-trips stay symmetric.
//
// Security: unlike TOML and JSON, YAML supports anchors and aliases, which the parser
// expands during decode. A small hostile document can reference-expand into a much larger
// in-memory tree ("billion laughs"). This codec does not cap expansion, so callers must
// decode only size-bounded input (for example via fs bounded reads); do not feed unbounded
// or untrusted streams straight into DecodeValue.
type YAMLCodec struct{}

// NewYAMLCodec returns the built-in YAML codec.
func NewYAMLCodec() YAMLCodec { return YAMLCodec{} }

// Name reports the codec identifier.
func (YAMLCodec) Name() string { return "yaml" }

// EncodeValue serializes a value tree as YAML. The top-level value must be a mapping.
func (YAMLCodec) EncodeValue(value Value) (string, error) {
	if _, ok := value.(map[string]any); !ok {
		return "", apperrors.InvalidInput("codec",
			"failed to serialize value as YAML: top level must be a mapping")
	}
	data, err := yaml.Marshal(value)
	if err != nil {
		return "", apperrors.InvalidInput("codec", "failed to serialize value as YAML").WithCause(err)
	}
	return string(data), nil
}

// DecodeValue parses YAML text into a value tree. The top-level value must be a mapping.
func (YAMLCodec) DecodeValue(contents string) (Value, error) {
	var tree Value
	if err := yaml.Unmarshal([]byte(contents), &tree); err != nil {
		return nil, apperrors.InvalidInput("codec", "failed to parse YAML").WithCause(err)
	}
	if _, ok := tree.(map[string]any); !ok {
		return nil, apperrors.InvalidInput("codec", "YAML top level must be a mapping")
	}
	return tree, nil
}
