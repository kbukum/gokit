package local

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kbukum/gokit/logger"
	"github.com/kbukum/gokit/storage"
)

func init() {
	storage.RegisterFactory(storage.ProviderLocal, func(cfg storage.Config, providerCfg any, log *logger.Logger) (storage.Storage, error) {
		c := &Config{}
		if providerCfg != nil {
			pc, ok := providerCfg.(*Config)
			if !ok {
				return nil, fmt.Errorf("local: expected *local.Config, got %T", providerCfg)
			}
			c = pc
		}
		c.ApplyDefaults()
		if err := c.Validate(); err != nil {
			return nil, err
		}
		return NewStorage(c.BasePath)
	})
}

// Storage implements storage.Storage using the local filesystem.
type Storage struct {
	basePath string
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
	return &Storage{basePath: abs}, nil
}

// Upload writes data from reader to a local file.
func (s *Storage) Upload(_ context.Context, path string, reader io.Reader) error {
	fullPath := filepath.Join(s.basePath, filepath.Clean(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o750); err != nil {
		return fmt.Errorf("storage: create directory: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("storage: create file: %w", err)
	}
	defer f.Close() //nolint:errcheck // Error on close is safe to ignore for read operations

	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("storage: write file: %w", err)
	}
	return nil
}

// Download returns a reader for the local file at the given path.
func (s *Storage) Download(_ context.Context, path string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.basePath, filepath.Clean(path))
	f, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("storage: file not found: %s", path)
		}
		return nil, fmt.Errorf("storage: open file: %w", err)
	}
	return f, nil
}

// Delete removes a local file. Returns nil if the file does not exist.
func (s *Storage) Delete(_ context.Context, path string) error {
	fullPath := filepath.Join(s.basePath, path)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete file: %w", err)
	}
	return nil
}

// Exists checks whether a local file exists.
func (s *Storage) Exists(_ context.Context, path string) (bool, error) {
	fullPath := filepath.Join(s.basePath, path)
	_, err := os.Stat(fullPath)
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
	fullPath := filepath.Join(s.basePath, path)
	u := &url.URL{Scheme: "file", Path: fullPath}
	return u.String(), nil
}

// List returns metadata for all files whose relative path starts with prefix.
func (s *Storage) List(_ context.Context, prefix string) ([]storage.FileInfo, error) {
	prefixPath := filepath.Join(s.basePath, prefix)
	baseDir := filepath.Dir(prefixPath)

	var files []storage.FileInfo

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
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
