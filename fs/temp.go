package fs

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

// nextTempSequence provides a per-process monotonic counter for collision-
// resistant temp path generation.
var nextTempSequence atomic.Uint64

// TempFile is a managed temporary file. Call [TempFile.Remove] to delete it, or
// [TempFile.Persist] to move it to a permanent location. Go has no destructors,
// so cleanup is explicit — pair creation with a deferred Remove.
type TempFile struct {
	file      *os.File
	path      string
	persisted bool
}

// NewTempFile creates a temporary file in the system temp directory.
func NewTempFile() (*TempFile, error) {
	return NewTempFileIn("")
}

// NewTempFileIn creates a temporary file in dir (the system temp directory when
// dir is empty).
func NewTempFileIn(dir string) (*TempFile, error) {
	file, err := os.CreateTemp(dir, "gokit-fs-*")
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to create temp file: %v", err),
			http.StatusInternalServerError).WithCause(err)
	}
	return &TempFile{file: file, path: file.Name()}, nil
}

// Path returns the temporary file's path.
func (t *TempFile) Path() string { return t.path }

// File returns the open file handle for writing.
func (t *TempFile) File() *os.File { return t.file }

// Persist closes the file and moves it to target, canceling automatic cleanup.
func (t *TempFile) Persist(target string) (string, error) {
	if err := t.file.Close(); err != nil {
		return "", apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to close temp file: %v", err),
			http.StatusInternalServerError).WithCause(err)
	}
	if err := os.Rename(t.path, target); err != nil {
		return "", apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to persist temp file to '%s': %v", target, err),
			http.StatusInternalServerError).WithCause(err)
	}
	t.persisted = true
	return target, nil
}

// Remove closes and deletes the temporary file. It is a no-op after a successful
// Persist.
func (t *TempFile) Remove() error {
	if t.persisted {
		return nil
	}
	_ = t.file.Close()
	if err := os.Remove(t.path); err != nil && !os.IsNotExist(err) {
		return apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to remove temp file '%s': %v", t.path, err),
			http.StatusInternalServerError).WithCause(err)
	}
	return nil
}

// TempDir is a managed temporary directory. Call [TempDir.Remove] to delete it
// and all of its contents.
type TempDir struct {
	path string
}

// NewTempDir creates a temporary directory in the system temp directory.
func NewTempDir() (*TempDir, error) {
	path, err := os.MkdirTemp("", "gokit-fs-*")
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to create temp dir: %v", err),
			http.StatusInternalServerError).WithCause(err)
	}
	return &TempDir{path: path}, nil
}

// Path returns the temporary directory's path.
func (d *TempDir) Path() string { return d.path }

// Child resolves relPath under the directory, rejecting traversal via [SafeJoin].
func (d *TempDir) Child(relPath string) (string, error) {
	return SafeJoin(d.path, relPath)
}

// WriteFile writes content to relPath under the directory, creating parent
// directories as needed, and returns the written path.
func (d *TempDir) WriteFile(relPath string, content []byte) (string, error) {
	target, err := d.Child(relPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return "", apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to create parent directories for '%s': %v", target, err),
			http.StatusInternalServerError).WithCause(err)
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return "", apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to write '%s': %v", target, err),
			http.StatusInternalServerError).WithCause(err)
	}
	return target, nil
}

// Remove deletes the temporary directory and all of its contents.
func (d *TempDir) Remove() error {
	if err := os.RemoveAll(d.path); err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to remove temp dir '%s': %v", d.path, err),
			http.StatusInternalServerError).WithCause(err)
	}
	return nil
}

// SiblingTempPath builds a collision-resistant temp path next to dest. The
// prefix and suffix are sanitized so the generated file name stays a single path
// component under dest's parent directory. It only constructs a path; the caller
// owns creation, writes, syncing, and any final rename policy.
func SiblingTempPath(dest, prefix, suffix string) string {
	parent, ok := ParentDir(dest)
	if !ok {
		parent = "."
	}
	prefix = sanitizeTempAffix(prefix, false)
	suffix = sanitizeTempAffix(suffix, true)
	sequence := nextTempSequence.Add(1)
	name := fmt.Sprintf(".%s-%d-%d-%d%s", prefix, os.Getpid(), time.Now().UnixNano(), sequence, suffix)
	return filepath.Join(parent, name)
}

func sanitizeTempAffix(value string, allowDot bool) string {
	var b strings.Builder
	b.Grow(len(value))
	previousDot := false
	for _, r := range value {
		var replacement rune
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			replacement = r
		case r == '.' && allowDot && !previousDot:
			replacement = '.'
		default:
			replacement = '_'
		}
		previousDot = replacement == '.'
		b.WriteRune(replacement)
	}
	return b.String()
}
