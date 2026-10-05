package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/kbukum/gokit/bench"
	gofs "github.com/kbukum/gokit/fs"
	gostorage "github.com/kbukum/gokit/storage"
)

// Option configures the storage adapter namespace.
type Option func(*ProviderStorage)

// WithPrefix sets the caller-owned object namespace, including its trailing separator.
func WithPrefix(prefix string) Option { return func(s *ProviderStorage) { s.prefix = prefix } }

// ProviderStorage adapts kit storage to the bounded benchmark object port.
type ProviderStorage struct {
	store  gostorage.Storage
	prefix string
}

// NewProviderStorage wraps a backend with the default "bench/" namespace.
func NewProviderStorage(store gostorage.Storage, opts ...Option) *ProviderStorage {
	s := &ProviderStorage{store: store, prefix: "bench/"}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *ProviderStorage) key(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("bench/storage: object key must not be empty")
	}
	if err := gofs.ValidateRelativePath(key); err != nil {
		return "", err
	}
	return s.prefix + key, nil
}

func (s *ProviderStorage) Put(ctx context.Context, key string, data []byte) error {
	path, err := s.key(key)
	if err != nil {
		return err
	}
	return s.store.Upload(ctx, path, bytes.NewReader(data))
}

func (s *ProviderStorage) Get(ctx context.Context, key string, limit int64) ([]byte, error) {
	if limit < 1 {
		return nil, fmt.Errorf("bench/storage: positive read limit required")
	}
	path, err := s.key(key)
	if err != nil {
		return nil, err
	}
	reader, err := s.store.Download(ctx, path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, limit+1))
	if err := errors.Join(readErr, reader.Close(), ctx.Err()); err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("bench/storage: object exceeds read limit %d", limit)
	}
	return data, nil
}

func (s *ProviderStorage) Delete(ctx context.Context, key string) error {
	path, err := s.key(key)
	if err != nil {
		return err
	}
	return s.store.Delete(ctx, path)
}

func (s *ProviderStorage) List(ctx context.Context, prefix string) ([]string, error) {
	objects, err := s.store.List(ctx, s.prefix+prefix)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(objects))
	for _, object := range objects {
		if !strings.HasPrefix(object.Path, s.prefix+prefix) {
			return nil, fmt.Errorf("bench/storage: backend returned object outside prefix")
		}
		keys = append(keys, strings.TrimPrefix(object.Path, s.prefix))
	}
	slices.Sort(keys)
	return keys, nil
}

var _ bench.ObjectStore = (*ProviderStorage)(nil)
