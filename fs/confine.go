package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
)

// ConfineExistingPath canonicalizes an existing path and rejects it when it resolves outside root.
// Use it for existing untrusted paths before handing them to lower-level IO or subprocess APIs:
// both root and path are resolved through the filesystem so symlink escapes are rejected.
// Relative paths are interpreted under root;
// absolute paths are accepted only when their canonical destination stays within root.
func ConfineExistingPath(root, path string) (string, error) {
	croot, err := canonicalizeDirectoryRoot(root)
	if err != nil {
		return "", err
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(croot, candidate)
	}
	resolved, err := canonicalizeConfinedInput(candidate, "confined path")
	if err != nil {
		return "", err
	}
	if err := ensureConfined(croot, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

// ConfinePath resolves path under root and rejects escapes, allowing the final path to be missing.
// It is intended for output paths:
// the nearest existing ancestor is canonicalized to catch symlink escapes before new directories
// or files are created.
func ConfinePath(root, path string) (string, error) {
	croot, err := canonicalizeDirectoryRoot(root)
	if err != nil {
		return "", err
	}
	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(croot, candidate)
	}
	existing, missing, err := existingAncestorAndMissingSuffix(candidate)
	if err != nil {
		return "", err
	}
	resolvedExisting, err := canonicalizeConfinedInput(existing, "existing path ancestor")
	if err != nil {
		return "", err
	}
	if cerr := ensureConfined(croot, resolvedExisting); cerr != nil {
		return "", cerr
	}
	if derr := ensureDirectoryForMissingSuffix(resolvedExisting, missing); derr != nil {
		return "", derr
	}
	resolved, err := appendSafeMissingSuffix(resolvedExisting, missing)
	if err != nil {
		return "", err
	}
	if cerr := ensureConfined(croot, resolved); cerr != nil {
		return "", cerr
	}
	return resolved, nil
}

func existingAncestorAndMissingSuffix(path string) (existing string, missing []string, err error) {
	current := path
	for {
		exists, existsErr := existsWithoutFollowingSymlinks(current)
		if existsErr != nil {
			return "", nil, existsErr
		}
		if exists {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil, apperrors.New(apperrors.ErrCodeNotFound,
				"no existing path ancestor").WithCause(fmt.Errorf("find ancestor of %q: %w", path, os.ErrNotExist))
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
	reverse(missing)
	return current, missing, nil
}

func canonicalizeDirectoryRoot(root string) (string, error) {
	resolved, err := canonicalizeConfinedInput(root, "confined root")
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", apperrors.New(apperrors.ErrCodeInternal,
			"failed to inspect confined root",
		).WithCause(err)
	}
	if !info.IsDir() {
		return "", apperrors.InvalidInput("root",
			"confined root is not a directory").WithCause(fmt.Errorf("root %q is not a directory", resolved))
	}
	return resolved, nil
}

func canonicalizeConfinedInput(path, label string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		code := osErrorCode(err)
		return "", apperrors.New(code,
			"failed to canonicalize "+label).WithCause(err)
	}
	return filepath.Abs(resolved)
}

func existsWithoutFollowingSymlinks(path string) (bool, error) {
	if _, err := os.Lstat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, apperrors.New(apperrors.ErrCodeInternal,
			"failed to inspect path",
		).WithCause(err)
	}
	return true, nil
}

func ensureDirectoryForMissingSuffix(existing string, missing []string) error {
	if len(missing) == 0 {
		return nil
	}
	info, err := os.Stat(existing)
	if err != nil {
		return apperrors.New(apperrors.ErrCodeInternal,
			"failed to inspect existing path ancestor",
		).WithCause(err)
	}
	if !info.IsDir() {
		return apperrors.InvalidInput("path",
			"existing path ancestor is not a directory").WithCause(fmt.Errorf("ancestor %q is not a directory", existing))
	}
	return nil
}

func appendSafeMissingSuffix(base string, missing []string) (string, error) {
	for _, segment := range missing {
		if err := ValidateRelativePath(segment); err != nil {
			return "", apperrors.InvalidInput("path",
				"path segment is not safe").WithCause(fmt.Errorf("unsafe segment %q: %w", segment, err))
		}
		if segments := splitSegments(segment); len(segments) != 1 || segments[0] == "" || segments[0] == "." {
			return "", apperrors.InvalidInput("path",
				"path segment is not safe").WithCause(fmt.Errorf("unsafe segment %q", segment))
		}
		base = filepath.Join(base, segment)
	}
	return base, nil
}

// ensureConfined rejects a path that resolves outside root using a component-aware relative check (not a string prefix, which would treat "/rootx" as inside "/root").
func ensureConfined(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return apperrors.InvalidInput("path",
			"path resolves outside confined root").WithCause(fmt.Errorf("path %q outside root %q", path, root))
	}
	return nil
}

func reverse(items []string) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
