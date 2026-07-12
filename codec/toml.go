package codec

import (
	"github.com/pelletier/go-toml/v2"

	apperrors "github.com/kbukum/gokit/errors"
)

// TOMLCodec is the built-in TOML codec.
//
// It decodes TOML into the canonical [Value] tree and encodes a value tree back
// to TOML. The top-level value must be a table (TOML has no top-level scalar or
// array document), and a nil value has no TOML representation — both surface as
// typed errors rather than panics.
type TOMLCodec struct{}

// NewTOMLCodec returns the built-in TOML codec.
func NewTOMLCodec() TOMLCodec { return TOMLCodec{} }

// Name reports the codec identifier.
func (TOMLCodec) Name() string { return "toml" }

// EncodeValue serializes a value tree as TOML.
func (TOMLCodec) EncodeValue(value Value) (string, error) {
	data, err := toml.Marshal(value)
	if err != nil {
		return "", apperrors.InvalidInput("codec", "failed to serialize value as TOML").WithCause(err)
	}
	return string(data), nil
}

// DecodeValue parses TOML text into a value tree.
func (TOMLCodec) DecodeValue(contents string) (Value, error) {
	var tree Value
	if err := toml.Unmarshal([]byte(contents), &tree); err != nil {
		return nil, apperrors.InvalidInput("codec", "failed to parse TOML").WithCause(err)
	}
	return tree, nil
}
