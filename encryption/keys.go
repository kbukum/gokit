package encryption

import (
	"context"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/util"
)

// KeySource resolves an encryption key by immutable operator-assigned identifier. Implementations must not log raw keys; callers expose a value only when constructing an encryptor.
type KeySource interface {
	Key(context.Context, string) (util.SecretString, error)
}

// KeyFile maps one key identifier to an operator-mounted secret file.
type KeyFile struct {
	ID   string `json:"id" yaml:"id" mapstructure:"id"`
	Path string `json:"path" yaml:"path" mapstructure:"path"`
}

// FileKeys reads bounded mounted keys for each operation, without a positive key cache.
type FileKeys struct {
	files map[string]string
}

var _ KeySource = (*FileKeys)(nil)

// NewFileKeys copies explicit references without performing I/O. Missing or invalid files fail on Key.
func NewFileKeys(files []KeyFile) (*FileKeys, error) {
	if len(files) == 0 {
		return nil, apperrors.InvalidInput("encryption keys", "At least one mounted key reference is required")
	}
	paths := make(map[string]string, len(files))
	for _, file := range files {
		if file.ID == "" || len(file.ID) > 128 || strings.TrimSpace(file.ID) != file.ID || file.Path == "" {
			return nil, apperrors.InvalidInput("encryption keys", "Each reference requires a bounded identifier and mounted path")
		}
		if _, exists := paths[file.ID]; exists {
			return nil, apperrors.InvalidInput("encryption keys", "Key identifiers must be unique")
		}
		paths[file.ID] = file.Path
	}
	return &FileKeys{files: paths}, nil
}

// Key reads a regular secret file of 32–1024 bytes. Errors preserve causes and never contain key contents.
func (s *FileKeys) Key(ctx context.Context, id string) (util.SecretString, error) {
	if util.IsNil(ctx) {
		return util.SecretString{}, apperrors.InvalidInput("encryption key", "A context is required")
	}
	if err := ctx.Err(); err != nil {
		return util.SecretString{}, err
	}
	path, ok := s.files[id]
	if !ok {
		return util.SecretString{}, apperrors.NotFound("encryption key", "").WithReason("ENCRYPTION_KEY_NOT_FOUND")
	}
	data, err := fs.ReadSecret(path, fs.SecretBounds{Min: 32, Max: 1024})
	if err != nil {
		return util.SecretString{}, apperrors.ServiceUnavailable("encryption key").WithReason("ENCRYPTION_KEY_UNAVAILABLE").WithCause(err)
	}
	if err := ctx.Err(); err != nil {
		return util.SecretString{}, err
	}
	return util.NewSecretString(string(data)), nil
}
