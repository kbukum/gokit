package bench

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	gofs "github.com/kbukum/gokit/fs"
)

// ObjectStore is the bounded object port used by ResultStore. Put replaces atomically.
type ObjectStore interface {
	Put(context.Context, string, []byte) error
	Get(context.Context, string, int64) ([]byte, error)
	Delete(context.Context, string) error
	List(context.Context, string) ([]string, error)
}

// DirStore stores private, atomically replaced objects under one directory.
type DirStore struct{ dir string }

// NewDirStore creates a store rooted at dir, creating it on first write.
func NewDirStore(dir string) *DirStore { return &DirStore{dir: dir} }

func (s *DirStore) Put(ctx context.Context, key string, data []byte) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if key == "" || key == "." {
		return fmt.Errorf("bench: nonempty object key required")
	}
	if err := gofs.ValidateRelativePath(key); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	path, err := gofs.ConfinePath(s.dir, key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return errors.Join(gofs.WriteAtomic(path, data, ".bench-"), ctx.Err())
}

func (s *DirStore) Get(ctx context.Context, key string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, fmt.Errorf("bench: positive read limit required")
	}
	path, err := gofs.ConfineExistingPath(s.dir, key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, limit+1))
	if err := errors.Join(readErr, f.Close(), ctx.Err()); err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("bench: object exceeds read limit %d", limit)
	}
	return data, nil
}

func (s *DirStore) Delete(ctx context.Context, key string) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if key == "" || key == "." {
		return fmt.Errorf("bench: nonempty object key required")
	}
	path, err := gofs.ConfinePath(s.dir, key)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (s *DirStore) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string
	err := filepath.WalkDir(s.dir, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && path == s.dir {
			return nil
		}
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if d.IsDir() {
			return nil
		}
		key, err := filepath.Rel(s.dir, path)
		if err != nil {
			return err
		}
		key = filepath.ToSlash(key)
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	slices.Sort(keys)
	return keys, err
}

// MemoryStore is an isolated object store for small runs and tests.
type MemoryStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

// NewMemoryStore creates an isolated in-memory store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{objects: make(map[string][]byte)} }

func (s *MemoryStore) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = bytes.Clone(data)
	return nil
}

func (s *MemoryStore) Get(ctx context.Context, key string, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.objects[key]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if limit < 1 || int64(len(data)) > limit {
		return nil, fmt.Errorf("bench: object exceeds read limit")
	}
	return bytes.Clone(data), nil
}

func (s *MemoryStore) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *MemoryStore) List(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var keys []string
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys, nil
}
