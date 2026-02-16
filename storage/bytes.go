// Package storage provides byte-oriented convenience types that wrap the
// streaming Storage interface with []byte operations.
package storage

import (
	"bytes"
	"context"
	"io"
)

// ObjectInfo contains minimal metadata about a stored object.
// This is a simplified alternative to FileInfo for callers that
// only need key and size information.
type ObjectInfo struct {
	Key  string // Object path/key
	Size int64  // Size in bytes
}

// ByteClient provides a []byte-oriented interface for storage operations.
// This is useful for callers that work with in-memory data rather than streams.
type ByteClient interface {
	// Upload stores data at the given path.
	Upload(ctx context.Context, path string, data []byte) error

	// Download retrieves data from the given path.
	Download(ctx context.Context, path string) ([]byte, error)

	// Delete removes the object at the given path.
	Delete(ctx context.Context, path string) error

	// Exists checks whether an object exists at the given path.
	Exists(ctx context.Context, path string) (bool, error)

	// List returns metadata for all objects whose path starts with prefix.
	List(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// byteAdapter wraps a streaming Storage and implements ByteClient.
type byteAdapter struct {
	storage Storage
}

// NewByteClient wraps a streaming Storage implementation with []byte convenience methods.
func NewByteClient(s Storage) ByteClient {
	return &byteAdapter{storage: s}
}

func (a *byteAdapter) Upload(ctx context.Context, path string, data []byte) error {
	return a.storage.Upload(ctx, path, bytes.NewReader(data))
}

func (a *byteAdapter) Download(ctx context.Context, path string) ([]byte, error) {
	rc, err := a.storage.Download(ctx, path)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (a *byteAdapter) Delete(ctx context.Context, path string) error {
	return a.storage.Delete(ctx, path)
}

func (a *byteAdapter) Exists(ctx context.Context, path string) (bool, error) {
	return a.storage.Exists(ctx, path)
}

func (a *byteAdapter) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	files, err := a.storage.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	objects := make([]ObjectInfo, len(files))
	for i, f := range files {
		objects[i] = ObjectInfo{
			Key:  f.Path,
			Size: f.Size,
		}
	}
	return objects, nil
}
