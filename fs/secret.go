package fs

import (
	"bytes"
	"fmt"

	apperrors "github.com/kbukum/gokit/errors"
)

// MaxSecretBytes bounds a mounted secret file's content.
const MaxSecretBytes = 64 << 10

// ReadSecret reads a bounded regular file and removes one trailing LF or CRLF. It follows symlinks, as mounted secrets may use them. Errors never contain file contents.
func ReadSecret(path string, minimum, limit int) ([]byte, error) {
	if path == "" || minimum < 1 || limit < minimum || limit > MaxSecretBytes {
		return nil, apperrors.InvalidInput("secret", "A path and valid secret bounds are required")
	}
	data, err := ReadFileLimit(path, int64(limit)+2)
	if err != nil {
		return nil, apperrors.New(apperrors.Normalize(err).Code, "Failed to read secret file").WithCause(fmt.Errorf("secret file %s: %w", path, err))
	}
	if trimmed, ok := bytes.CutSuffix(data, []byte("\n")); ok {
		data, _ = bytes.CutSuffix(trimmed, []byte("\r"))
	}
	if len(data) < minimum || len(data) > limit {
		return nil, apperrors.InvalidInput("secret", fmt.Sprintf("Secret file %s must contain %d to %d bytes", path, minimum, limit))
	}
	return data, nil
}
