package codec

import (
	"encoding/json"

	apperrors "github.com/kbukum/gokit/errors"
)

// JSONStyle selects the output layout for [JSONCodec].
type JSONStyle int

const (
	// JSONStylePretty emits human-readable, indented output. It is the default.
	JSONStylePretty JSONStyle = iota
	// JSONStyleCompact emits minimal single-line output for machine streams (newline-delimited JSON, length-framed payloads) where size and one value per line matter.
	JSONStyleCompact
)

// JSONCodec is the built-in JSON codec, always available because JSON backs the
// package's value model. It defaults to pretty-printed output.
type JSONCodec struct {
	style JSONStyle
}

// PrettyJSON returns a pretty-printing JSON codec.
func PrettyJSON() JSONCodec { return JSONCodec{style: JSONStylePretty} }

// CompactJSON returns a compact (single-line) JSON codec for machine streams.
func CompactJSON() JSONCodec { return JSONCodec{style: JSONStyleCompact} }

// Name reports the codec identifier.
func (JSONCodec) Name() string { return "json" }

// EncodeValue serializes a value tree as JSON in the codec's style.
func (c JSONCodec) EncodeValue(value Value) (string, error) {
	var (
		data []byte
		err  error
	)
	switch c.style {
	case JSONStyleCompact:
		data, err = json.Marshal(value)
	default:
		data, err = json.MarshalIndent(value, "", "  ")
	}
	if err != nil {
		return "", apperrors.InvalidInput("codec", "failed to serialize value as JSON").WithCause(err)
	}
	return string(data), nil
}

// DecodeValue parses JSON text into a value tree.
func (JSONCodec) DecodeValue(contents string) (Value, error) {
	var tree Value
	if err := json.Unmarshal([]byte(contents), &tree); err != nil {
		return nil, apperrors.InvalidInput("codec", "failed to parse JSON").WithCause(err)
	}
	return tree, nil
}
