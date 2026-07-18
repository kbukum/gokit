package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
)

// Sentinel errors returned by the pure path-validation helpers.
// They are plain sentinel errors (not AppError) so callers can match with errors.Is;
// the filesystem-touching helpers return typed AppError values instead.
var (
	// ErrPathAbsolute means an absolute path was given where a root-relative one is required.
	ErrPathAbsolute = errors.New("path must be relative, not absolute")
	// ErrPathParentDir means the path contains a '..' traversal segment.
	ErrPathParentDir = errors.New("path must not contain '..' segments")
	// ErrPathVolumeName means the path carries a platform volume name (for example a Windows drive letter).
	ErrPathVolumeName = errors.New("path must not contain a volume name")
	// ErrPathEmpty means the path has no components.
	ErrPathEmpty = errors.New("path must not be empty")
)

// ValidateRelativePath reports whether path is safe to join under a caller-owned root directory,
// rejecting absolute paths, '..' traversal, and volume names.
func ValidateRelativePath(path string) error {
	if filepath.IsAbs(path) {
		return ErrPathAbsolute
	}
	if filepath.VolumeName(path) != "" {
		return ErrPathVolumeName
	}
	for _, segment := range splitSegments(path) {
		if segment == ".." {
			return ErrPathParentDir
		}
	}
	return nil
}

// SafeJoin joins a caller-owned root with a validated relative path.
func SafeJoin(root, relPath string) (string, error) {
	if err := ValidateRelativePath(relPath); err != nil {
		return "", err
	}
	return filepath.Join(root, relPath), nil
}

// NormalizeRelativePath strips '.' components
// so semantically equal inputs share one canonical value, while rejecting empty, absolute,
// volume-prefixed, or '..' paths. A path consisting solely of '.' components collapses to ".".
func NormalizeRelativePath(path string) (string, error) {
	if path == "" {
		return "", ErrPathEmpty
	}
	if err := ValidateRelativePath(path); err != nil {
		return "", err
	}
	var parts []string
	for _, segment := range splitSegments(path) {
		switch segment {
		case "", ".":
		case "..":
			return "", ErrPathParentDir
		default:
			parts = append(parts, segment)
		}
	}
	if len(parts) == 0 {
		return ".", nil
	}
	return filepath.Join(parts...), nil
}

// Absolute returns an absolute path without requiring the path to exist.
func Absolute(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to make path absolute: %v", err), 500).WithCause(err)
	}
	return abs, nil
}

// Canonicalize resolves symlinks and normalizes components, requiring the path to exist.
func Canonicalize(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", canonicalizeError(path, err)
	}
	return filepath.Abs(resolved)
}

// FindInAncestors searches start and each ancestor directory for a regular file named fileName,
// returning the nearest match. A nested directory's file shadows one higher up.
// The boolean is false when no ancestor contains the file.
func FindInAncestors(start, fileName string) (string, bool) {
	dir := start
	for {
		candidate := filepath.Join(dir, fileName)
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// ParentDir returns the parent directory of path,
// or false when path has no distinct parent (a filesystem root or ".").
func ParentDir(path string) (string, bool) {
	parent := filepath.Dir(path)
	if parent == path {
		return "", false
	}
	return parent, true
}

// splitSegments splits path into its components using the OS separator,
// normalizing backslashes on Windows via filepath.ToSlash.
func splitSegments(path string) []string {
	return strings.Split(filepath.ToSlash(path), "/")
}

func canonicalizeError(path string, err error) error {
	code, status := osErrorCode(err)
	return apperrors.New(code,
		fmt.Sprintf("failed to canonicalize '%s': %v", path, err), status).WithCause(err)
}
