// Package storage provides interfaces and implementations for object storage. Supported providers:
// local filesystem, Amazon S3 (and S3-compatible services).
package storage

import (
	"context"
	"io"
	"time"
)

// Storage defines the interface for object storage operations.
type Storage interface {
	// Upload writes data from reader to the given path.
	Upload(ctx context.Context, path string, reader io.Reader) error

	// Download returns a reader for the object at the given path.
	// The caller is responsible for closing the returned ReadCloser.
	Download(ctx context.Context, path string) (io.ReadCloser, error)

	// Delete removes the object at the given path. Returns nil if the object does not exist.
	Delete(ctx context.Context, path string) error

	// Exists checks whether an object exists at the given path.
	Exists(ctx context.Context, path string) (bool, error)

	// URL returns a public URL for accessing the object at the given path.
	URL(ctx context.Context, path string) (string, error)

	// List returns metadata for all objects whose path starts with prefix.
	List(ctx context.Context, prefix string) ([]FileInfo, error)
}

// HeadProvider is optionally implemented by storage backends that can return metadata for a single object more efficiently than scanning a listing. Use [Head] to call it with a listing-based fallback.
type HeadProvider interface {
	// Head returns metadata for the single object at path, including its size,
	// last-modified time, content type, and any metadata or checksum the backend
	// provides. It returns a not-found error when no object exists at path.
	Head(ctx context.Context, path string) (FileInfo, error)
}

// CopyProvider is optionally implemented by storage backends that support a server-side copy within the store. Use [Copy] to call it with a streaming fallback.
type CopyProvider interface {
	// Copy duplicates the object at srcPath to dstPath within the store,
	// leaving the source in place.
	Copy(ctx context.Context, srcPath, dstPath string) error
}

// RenameProvider is optionally implemented by storage backends that support a server-side move within the store. Use [Rename] to call it with a copy-then-delete fallback.
type RenameProvider interface {
	// Rename moves the object at srcPath to dstPath within the store,
	// removing the source once the destination is written.
	Rename(ctx context.Context, srcPath, dstPath string) error
}

// SignedURLProvider is optionally implemented by storage backends that support generating time-limited signed URLs for private object access.
type SignedURLProvider interface {
	// SignedURL returns a pre-signed URL valid for the specified duration.
	SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error)
}
