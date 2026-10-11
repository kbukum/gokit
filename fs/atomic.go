package fs

import (
	"errors"
	"fmt"
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
	return writeAtomic(dest, bytes, tempPrefix, false, false)
}

// WriteAtomicReplace writes bytes to dest and replaces an existing destination.
// Replacement is atomic on Unix-like platforms;
// on Windows the existing file is removed before the rename because the platform rename cannot replace it.
func WriteAtomicReplace(dest string, bytes []byte, tempPrefix string) error {
	return writeAtomic(dest, bytes, tempPrefix, true, false)
}

// WriteAtomicCreate publishes a complete owner-only file without replacing an existing destination, including a symlink.
// Its parent must exist. It uses a sibling temporary file and a hard link; filesystems without hard-link support return an error.
func WriteAtomicCreate(dest string, bytes []byte, tempPrefix string) error {
	return writeAtomic(dest, bytes, tempPrefix, false, true)
}

func writeAtomic(dest string, bytes []byte, tempPrefix string, replaceExisting, createOnly bool) error {
	if !createOnly {
		if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
			return apperrors.New(apperrors.ErrCodeInternal,
				"failed to create parent directories",
			).WithCause(err)
		}
	}

	for attempt := 0; attempt < writeAtomicAttempts; attempt++ {
		tempPath := SiblingTempPath(dest, tempPrefix, ".tmp")
		file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return apperrors.New(apperrors.ErrCodeInternal,
				"failed to create temp file",
			).WithCause(err)
		}
		if err := writeAndPersist(file, bytes, atomicTarget{tempPath: tempPath, dest: dest, replaceExisting: replaceExisting, createOnly: createOnly}); err != nil {
			return errors.Join(err, os.Remove(tempPath))
		}
		if createOnly {
			if err := os.Remove(tempPath); err != nil {
				return apperrors.Internal(fmt.Errorf("remove published file's temporary link: %w", err))
			}
		}
		return nil
	}

	return apperrors.New(apperrors.ErrCodeInternal,
		"failed to create a unique temp file",
	).WithCause(fmt.Errorf("create temp file for %q after %d attempts: %w", dest, writeAtomicAttempts, os.ErrExist))
}

// atomicTarget describes the destination of an atomic write for writeAndPersist.
type atomicTarget struct {
	tempPath        string
	dest            string
	replaceExisting bool
	createOnly      bool
}

func writeAndPersist(file *os.File, bytes []byte, target atomicTarget) error {
	if _, err := file.Write(bytes); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			"failed to write temp file",
		).WithCause(errors.Join(err, file.Close()))
	}
	if err := file.Sync(); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			"failed to sync temp file",
		).WithCause(errors.Join(err, file.Close()))
	}
	if err := file.Close(); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			"failed to close temp file",
		).WithCause(err)
	}
	if target.createOnly {
		if err := os.Link(target.tempPath, target.dest); err != nil {
			return apperrors.Internal(fmt.Errorf("publish new file: %w", err))
		}
		return nil
	}
	if target.replaceExisting && runtime.GOOS == "windows" {
		if err := os.Remove(target.dest); err != nil && !os.IsNotExist(err) {
			return apperrors.New(apperrors.ErrCodeInternal,
				"failed to remove existing destination",
			).WithCause(err)
		}
	}
	if err := os.Rename(target.tempPath, target.dest); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			"failed to rename temp file",
		).WithCause(err)
	}
	return nil
}
