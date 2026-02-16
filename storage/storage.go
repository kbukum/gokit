// Package storage provides interfaces and implementations for object storage.
// Supported providers: local filesystem, Amazon S3 (and S3-compatible services).
package storage

import (
	"context"
	"io"
	"time"
)

// FileInfo contains metadata about a stored object.
type FileInfo struct {
	Path         string
	Size         int64
	LastModified time.Time
	ContentType  string
}

// Storage defines the interface for object storage operations.
type Storage interface {
	// Upload writes data from reader to the given path.
	Upload(ctx context.Context, path string, reader io.Reader) error

	// Download returns a reader for the object at the given path.
	// The caller is responsible for closing the returned ReadCloser.
	Download(ctx context.Context, path string) (io.ReadCloser, error)

	// Delete removes the object at the given path.
	// Returns nil if the object does not exist.
	Delete(ctx context.Context, path string) error

	// Exists checks whether an object exists at the given path.
	Exists(ctx context.Context, path string) (bool, error)

	// URL returns a public URL for accessing the object at the given path.
	URL(ctx context.Context, path string) (string, error)

	// List returns metadata for all objects whose path starts with prefix.
	List(ctx context.Context, prefix string) ([]FileInfo, error)
}

// SignedURLProvider is optionally implemented by storage backends that support
// generating time-limited signed URLs for private object access.
type SignedURLProvider interface {
	// SignedURL returns a pre-signed URL valid for the specified duration.
	SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error)
}
