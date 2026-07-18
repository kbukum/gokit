package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

// FileMeta is metadata for a filesystem entry at a path, read without following symlinks.
type FileMeta struct {
	// Path is the inspected path.
	Path string
	// Len is the file size in bytes.
	Len int64
	// Modified is the last modification time.
	Modified time.Time
	// IsFile reports whether the path is a regular file.
	IsFile bool
	// IsDir reports whether the path is a directory.
	IsDir bool
	// IsSymlink reports whether the path is a symlink.
	IsSymlink bool
}

// DirEntry is metadata for an entry directly inside a directory.
type DirEntry struct {
	// Path is the entry path.
	Path string
	// Name is the entry file name.
	Name string
	// IsFile reports whether the entry is a regular file.
	IsFile bool
	// IsDir reports whether the entry is a directory.
	IsDir bool
	// IsSymlink reports whether the entry is a symlink.
	IsSymlink bool
}

// Metadata reads metadata for path without following symlinks.
func Metadata(path string) (FileMeta, error) {
	info, err := os.Lstat(path)
	if err != nil {
		code, status := osErrorCode(err)
		return FileMeta{}, apperrors.New(code,
			fmt.Sprintf("failed to inspect '%s': %v", path, err), status).WithCause(err)
	}
	return FileMeta{
		Path:      path,
		Len:       info.Size(),
		Modified:  info.ModTime(),
		IsFile:    info.Mode().IsRegular(),
		IsDir:     info.IsDir(),
		IsSymlink: info.Mode()&os.ModeSymlink != 0,
	}, nil
}

// ReadDir lists the entries directly inside dir without following symlinks.
func ReadDir(dir string) ([]DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		code, status := osErrorCode(err)
		return nil, apperrors.New(code,
			fmt.Sprintf("failed to read directory '%s': %v", dir, err), status).WithCause(err)
	}
	out := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		mode := entry.Type()
		out = append(out, DirEntry{
			Path:      filepath.Join(dir, entry.Name()),
			Name:      entry.Name(),
			IsFile:    mode.IsRegular(),
			IsDir:     entry.IsDir(),
			IsSymlink: mode&os.ModeSymlink != 0,
		})
	}
	return out, nil
}
