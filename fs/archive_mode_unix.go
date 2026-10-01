//go:build unix

package fs

import (
	"fmt"
	"os"

	apperrors "github.com/kbukum/gokit/errors"
)

// applyArchiveMode re-applies a recorded Unix mode to an extracted member.
func applyArchiveMode(archivePath, target string, mode os.FileMode) error {
	if mode == 0 {
		return nil
	}
	if err := os.Chmod(target, mode.Perm()); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			"cannot set archive member permissions",
		).WithCause(fmt.Errorf("set permissions on %q for %q: %w", target, archivePath, err))
	}
	return nil
}
