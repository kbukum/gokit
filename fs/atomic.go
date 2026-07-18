package fs

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	apperrors "github.com/kbukum/gokit/errors"
)

// writeAtomicAttempts bounds retries when a generated temp path collides.
const writeAtomicAttempts = 8

// WriteAtomic writes bytes to dest by writing a sibling temp file and renaming it,
// so a reader never observes a partial file. Parent directories are created as needed. On Unix,
// renaming replaces any existing destination atomically.
func WriteAtomic(dest string, bytes []byte, tempPrefix string) error {
	return writeAtomic(dest, bytes, tempPrefix, false)
}

// WriteAtomicReplace writes bytes to dest and replaces an existing destination.
// Replacement is atomic on Unix-like platforms;
// on Windows the existing file is removed before the rename because the platform rename cannot replace it.
func WriteAtomicReplace(dest string, bytes []byte, tempPrefix string) error {
	return writeAtomic(dest, bytes, tempPrefix, true)
}

func writeAtomic(dest string, bytes []byte, tempPrefix string, replaceExisting bool) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to create parent directories for '%s': %v", dest, err),
			http.StatusInternalServerError).WithCause(err)
	}

	for attempt := 0; attempt < writeAtomicAttempts; attempt++ {
		tempPath := SiblingTempPath(dest, tempPrefix, ".tmp")
		file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return apperrors.New(apperrors.ErrCodeInternal,
				fmt.Sprintf("failed to create temp file '%s': %v", tempPath, err),
				http.StatusInternalServerError).WithCause(err)
		}
		if err := writeAndPersist(file, tempPath, dest, bytes, replaceExisting); err != nil {
			_ = os.Remove(tempPath)
			return err
		}
		return nil
	}

	return apperrors.New(apperrors.ErrCodeInternal,
		fmt.Sprintf("failed to create a unique temp file for '%s' after %d attempts", dest, writeAtomicAttempts),
		http.StatusInternalServerError)
}

func writeAndPersist(file *os.File, tempPath, dest string, bytes []byte, replaceExisting bool) error {
	if _, err := file.Write(bytes); err != nil {
		_ = file.Close()
		return apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to write temp file '%s': %v", tempPath, err),
			http.StatusInternalServerError).WithCause(err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to sync temp file '%s': %v", tempPath, err),
			http.StatusInternalServerError).WithCause(err)
	}
	if err := file.Close(); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to close temp file '%s': %v", tempPath, err),
			http.StatusInternalServerError).WithCause(err)
	}
	if replaceExisting && runtime.GOOS == "windows" {
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			return apperrors.New(apperrors.ErrCodeInternal,
				fmt.Sprintf("failed to remove existing destination '%s': %v", dest, err),
				http.StatusInternalServerError).WithCause(err)
		}
	}
	if err := os.Rename(tempPath, dest); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to rename temp file to '%s': %v", dest, err),
			http.StatusInternalServerError).WithCause(err)
	}
	return nil
}
