package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/kbukum/gokit/fs"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/storage"
	"github.com/kbukum/gokit/util"
)

// safePath resolves path within basePath and ensures it cannot escape via traversal.
func safePath(basePath, path string) (string, error) {
	// Prefix with "/" so Clean removes leading "../" sequences
	fullPath := filepath.Join(basePath, filepath.Clean("/"+path))
	if !strings.HasPrefix(fullPath, basePath+string(filepath.Separator)) && fullPath != basePath {
		return "", fmt.Errorf("storage: path %q escapes base directory", path)
	}
	return fullPath, nil
}

// Register registers a configured local storage provider into the given registry.
// Pass an optional Config to override defaults.
func Register(reg *storage.FactoryRegistry, configs ...Config) error {
	if reg == nil {
		return fmt.Errorf("local: storage registry is nil")
	}
	if len(configs) > 1 {
		return fmt.Errorf("local: at most one config may be provided, got %d", len(configs))
	}
	c := Config{}
	if len(configs) > 0 {
		c = configs[0]
	}
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return err
	}
	return reg.Register(storage.ProviderLocal, func(_ storage.Config, _ *logging.Logger) (storage.Storage, error) {
		return NewStorage(c.BasePath)
	})
}

// Storage implements storage.Storage using the local filesystem.
type Storage struct {
	basePath string
	// renameFile performs the atomic move; it is a field so tests can exercise
	// the copy-then-delete fallback taken when a rename crosses a mount point.
	renameFile func(oldpath, newpath string) error
}

// NewStorage creates a new local filesystem storage.
func NewStorage(basePath string) (*Storage, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("storage: resolve base path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("storage: create base directory: %w", err)
	}
	return &Storage{basePath: abs, renameFile: os.Rename}, nil
}

// Upload writes data from reader to a local file.
func (s *Storage) Upload(_ context.Context, path string, reader io.Reader) error {
	fullPath, err := safePath(s.basePath, path)
	if err != nil {
		return err
	}
	if mErr := os.MkdirAll(filepath.Dir(fullPath), 0o750); mErr != nil {
		return fmt.Errorf("storage: create directory: %w", mErr)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("storage: create file: %w", err)
	}

	if _, err := io.Copy(f, reader); err != nil {
		_ = f.Close()
		return fmt.Errorf("storage: write file: %w", err)
	}
	return f.Close()
}

// Download returns a reader for the local file at the given path.
func (s *Storage) Download(_ context.Context, path string) (io.ReadCloser, error) {
	fullPath, err := safePath(s.basePath, path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.NotFoundError(path)
		}
		return nil, fmt.Errorf("storage: open file: %w", err)
	}
	return f, nil
}

// Delete removes a local file. Returns nil if the file does not exist.
func (s *Storage) Delete(_ context.Context, path string) error {
	fullPath, err := safePath(s.basePath, path)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete file: %w", err)
	}
	return nil
}

// Exists checks whether a local file exists.
func (s *Storage) Exists(_ context.Context, path string) (bool, error) {
	fullPath, err := safePath(s.basePath, path)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("storage: stat file: %w", err)
	}
	return true, nil
}

// URL returns a file:// URL for the local file.
func (s *Storage) URL(_ context.Context, path string) (string, error) {
	fullPath, err := safePath(s.basePath, path)
	if err != nil {
		return "", err
	}
	u := &url.URL{Scheme: "file", Path: fullPath}
	return u.String(), nil
}

// Head returns metadata for a single local file.
func (s *Storage) Head(_ context.Context, path string) (storage.FileInfo, error) {
	fullPath, err := safePath(s.basePath, path)
	if err != nil {
		return storage.FileInfo{}, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return storage.FileInfo{}, storage.NotFoundError(path)
		}
		return storage.FileInfo{}, fmt.Errorf("storage: stat file: %w", err)
	}
	if info.IsDir() {
		return storage.FileInfo{}, fmt.Errorf("storage: not a file: %s", path)
	}
	ct := mime.TypeByExtension(filepath.Ext(fullPath))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return storage.FileInfo{
		Path:         path,
		Size:         info.Size(),
		LastModified: info.ModTime(),
		ContentType:  ct,
	}, nil
}

// Copy duplicates the file at srcPath to dstPath, leaving the source in place.
func (s *Storage) Copy(_ context.Context, srcPath, dstPath string) error {
	if err := storage.CheckDistinctPaths("Copy", srcPath, dstPath); err != nil {
		return err
	}
	return s.copyFile(srcPath, dstPath)
}

// Rename moves the file at srcPath to dstPath. It uses an atomic os.Rename when
// possible and falls back to a copy followed by a delete only when the rename
// crosses a device boundary (syscall.EXDEV) that os.Rename cannot handle; any
// other rename error is returned so a permission or I/O failure never silently
// downgrades to a non-atomic, potentially destructive operation.
func (s *Storage) Rename(_ context.Context, srcPath, dstPath string) error {
	if err := storage.CheckDistinctPaths("Rename", srcPath, dstPath); err != nil {
		return err
	}
	srcFull, err := fs.ConfineExistingPath(s.basePath, srcPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storage.NotFoundError(srcPath)
		}
		return fmt.Errorf("storage: resolve source: %w", err)
	}
	dstFull, err := fs.ConfinePath(s.basePath, dstPath)
	if err != nil {
		return fmt.Errorf("storage: resolve destination: %w", err)
	}
	if mErr := os.MkdirAll(filepath.Dir(dstFull), 0o750); mErr != nil {
		return fmt.Errorf("storage: create directory: %w", mErr)
	}
	rErr := s.renameFile(srcFull, dstFull)
	if rErr == nil {
		return nil
	}
	if !errors.Is(rErr, syscall.EXDEV) {
		return fmt.Errorf("storage: rename file: %w", rErr)
	}
	if cErr := s.copyFile(srcPath, dstPath); cErr != nil {
		return cErr
	}
	if remErr := os.Remove(srcFull); remErr != nil {
		return fmt.Errorf("storage: delete source file: %w", remErr)
	}
	return nil
}

// copyFile copies srcPath to dstPath within the base directory. Both paths are
// resolved through the filesystem (fs.ConfineExistingPath / fs.ConfinePath) so a
// symlinked key cannot read from or overwrite a file outside basePath, and the
// source permissions and modification time are preserved.
func (s *Storage) copyFile(srcPath, dstPath string) error {
	srcFull, err := fs.ConfineExistingPath(s.basePath, srcPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storage.NotFoundError(srcPath)
		}
		return fmt.Errorf("storage: resolve source: %w", err)
	}
	dstFull, err := fs.ConfinePath(s.basePath, dstPath)
	if err != nil {
		return fmt.Errorf("storage: resolve destination: %w", err)
	}
	if cErr := util.CopyFile(srcFull, dstFull); cErr != nil {
		return fmt.Errorf("storage: copy file: %w", cErr)
	}
	return nil
}

// List returns metadata for all files whose relative path starts with prefix.
func (s *Storage) List(_ context.Context, prefix string) ([]storage.FileInfo, error) {
	prefixPath, err := safePath(s.basePath, prefix)
	if err != nil {
		return nil, err
	}
	baseDir := prefixPath

	var files []storage.FileInfo

	err = filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(s.basePath, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(relPath, prefix) || strings.HasPrefix(path, prefixPath) {
			ct := mime.TypeByExtension(filepath.Ext(path))
			if ct == "" {
				ct = "application/octet-stream"
			}
			files = append(files, storage.FileInfo{
				Path:         relPath,
				Size:         info.Size(),
				LastModified: info.ModTime(),
				ContentType:  ct,
			})
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return []storage.FileInfo{}, nil
		}
		return nil, fmt.Errorf("storage: list files: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return files, nil
}

// compile-time check
var _ storage.Storage = (*Storage)(nil)
