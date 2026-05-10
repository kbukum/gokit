package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyFile copies a single file from src to dst, preserving permissions and modification times.
// Parent directories of dst are created as needed.
func CopyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", src)
	}
	if ensureErr := EnsureDir(filepath.Dir(dst)); ensureErr != nil {
		return ensureErr
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}

type copiedDir struct {
	path string
	info os.FileInfo
}

// CopyDir recursively copies a directory tree from src to dst.
// It preserves file permissions, modification times, and symlinks.
// dst must not be inside src; passing an overlapping dst returns an error.
func CopyDir(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source is not a directory: %s", src)
	}

	absSrc, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	absDst, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	if absDst == absSrc || strings.HasPrefix(absDst+string(filepath.Separator), absSrc+string(filepath.Separator)) {
		return fmt.Errorf("destination %s is inside source %s", dst, src)
	}

	var dirs []copiedDir

	if err := filepath.Walk(src, func(path string, entry os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := dst
		if rel != "." {
			target = filepath.Join(dst, rel)
		}

		switch mode := entry.Mode(); {
		case entry.IsDir():
			if err := os.MkdirAll(target, mode.Perm()); err != nil {
				return err
			}
			dirs = append(dirs, copiedDir{path: target, info: entry})
			return nil
		case mode&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := EnsureDir(filepath.Dir(target)); err != nil {
				return err
			}
			if err := RemoveAll(target); err != nil {
				return err
			}
			return os.Symlink(link, target) //nolint:gosec // G122: TOCTOU acceptable — developer tooling copying local trees, not untrusted input
		default:
			return CopyFile(path, target)
		}
	}); err != nil {
		return err
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		dir := dirs[i]
		if err := os.Chmod(dir.path, dir.info.Mode().Perm()); err != nil {
			return err
		}
		if err := os.Chtimes(dir.path, dir.info.ModTime(), dir.info.ModTime()); err != nil {
			return err
		}
	}

	return nil
}

// EnsureDir creates a directory and all parents if they don't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o750)
}

// FileExists reports whether path exists and is a regular file.
func FileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// DirExists reports whether path exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// WriteFile writes content to path, creating parent directories as needed.
// Uses 0o644 permissions for the file.
func WriteFile(path string, data []byte) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ReadFileString reads a file and returns its content as a string.
func ReadFileString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RemoveAll removes path and any children. It does not return an error if path doesn't exist.
func RemoveAll(path string) error {
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
