package config

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/kbukum/gokit/codec"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
)

// maxStrictFileBytes bounds the size of a strictly loaded config file (4 MiB).
const maxStrictFileBytes int64 = 4 * 1024 * 1024

// LoadStrict reads path and decodes it into T, rejecting any key that does not map to
// a field of T. The codec is selected from the file extension. Strict loading turns a
// stray or misspelled key into an actionable error instead of silently ignoring it,
// so a typo in production config fails fast at load time.
func LoadStrict[T any](path string) (T, error) {
	var zero T
	c, ok := codec.CodecForPath(path)
	if !ok {
		return zero, apperrors.InvalidInput("config",
			fmt.Sprintf("no codec is registered for config file '%s'", path))
	}
	return LoadStrictWithCodec[T](path, c)
}

// LoadStrictWithCodec reads path with an explicit codec and decodes it into T,
// rejecting unknown keys. Unknown-key rejection is recursive: a stray key at any
// nesting level fails the load.
func LoadStrictWithCodec[T any](path string, c codec.Codec) (T, error) {
	var zero T
	contents, err := fs.ReadFileLimit(path, maxStrictFileBytes)
	if err != nil {
		return zero, err
	}
	value, err := c.DecodeValue(string(contents))
	if err != nil {
		return zero, apperrors.InvalidInput("config",
			fmt.Sprintf("failed to parse config file '%s'", path)).WithCause(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return zero, apperrors.InvalidInput("config",
			fmt.Sprintf("failed to normalize config file '%s'", path)).WithCause(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var result T
	if err := decoder.Decode(&result); err != nil {
		return zero, apperrors.InvalidInput("config",
			fmt.Sprintf("config file '%s' has invalid or unknown keys", path)).WithCause(err)
	}
	return result, nil
}
