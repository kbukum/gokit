package codec

import (
	"encoding/json"

	apperrors "github.com/kbukum/gokit/errors"
)

// Value is the canonical in-memory value tree shared by every [Codec].
//
// It is the untyped result of decoding JSON — nested map[string]any, []any,
// float64, string, bool, and nil. Using any here is a deliberate, documented
// exception to the no-any rule: the tree carries genuinely heterogeneous
// document data whose leaf values cannot be given a closed type.
type Value = any

// Codec encodes and decodes one structured-text format over the [Value] model.
//
// Implementations translate a single on-disk/text representation (JSON, TOML, …)
// to and from [Value]. The interface is intentionally small so a codec can be
// held as an interface value and selected at runtime; type-driven conversions
// live in the free functions [Encode] and [Decode].
type Codec interface {
	// Name returns a short identifier for diagnostics (for example "toml").
	Name() string
	// EncodeValue encodes a value tree into this codec's textual representation,
	// returning a typed error (cause preserved) when value cannot be represented.
	EncodeValue(value Value) (string, error)
	// DecodeValue decodes text into a value tree, returning a typed error (cause
	// preserved) when contents is malformed for this format.
	DecodeValue(contents string) (Value, error)
}

// Encode encodes value using codec by first routing it through the [Value] tree.
//
// It returns a typed error (cause preserved) when value cannot be converted to
// the value model or encoded by codec.
func Encode[T any](codec Codec, value T) (string, error) {
	tree, err := toValue(value)
	if err != nil {
		return "", err
	}
	return codec.EncodeValue(tree)
}

// Decode decodes contents into T using codec by routing through the [Value] tree.
//
// It returns a typed error (cause preserved) when contents is malformed or does
// not match T.
func Decode[T any](codec Codec, contents string) (T, error) {
	var out T
	tree, err := codec.DecodeValue(contents)
	if err != nil {
		return out, err
	}
	data, err := json.Marshal(tree)
	if err != nil {
		return out, apperrors.InvalidInput("codec", "failed to convert value into the value model").WithCause(err)
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, apperrors.InvalidInput("codec", "failed to deserialize value").WithCause(err)
	}
	return out, nil
}

// toValue converts an arbitrary Go value into the canonical [Value] tree.
func toValue[T any](value T) (Value, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, apperrors.InvalidInput("codec", "failed to convert value into the value model").WithCause(err)
	}
	var tree Value
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, apperrors.InvalidInput("codec", "failed to convert value into the value model").WithCause(err)
	}
	return tree, nil
}
