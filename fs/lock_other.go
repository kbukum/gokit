//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !windows

package fs

import (
	"errors"
	"os"
)

func tryFileLock(*os.File) (bool, error) {
	return false, errors.ErrUnsupported
}

func unlockFile(*os.File) error { return errors.ErrUnsupported }
