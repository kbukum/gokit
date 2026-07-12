//go:build unix

package fs

import (
	"fmt"
	"net/http"
	"os"

	apperrors "github.com/kbukum/gokit/errors"
)

// Mode reads a path's Unix permission bits.
func Mode(path string) (os.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, accessError("read permissions", path, err)
	}
	return info.Mode().Perm(), nil
}

// SetMode sets a path's Unix permission bits.
func SetMode(path string, mode os.FileMode) error {
	if err := os.Chmod(path, mode.Perm()); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to set permissions for '%s': %v", path, err),
			http.StatusInternalServerError).WithCause(err)
	}
	return nil
}

// IsExecutable reports whether path has any executable bit set.
func IsExecutable(path string) (bool, error) {
	mode, err := Mode(path)
	if err != nil {
		return false, err
	}
	return mode&0o111 != 0, nil
}
