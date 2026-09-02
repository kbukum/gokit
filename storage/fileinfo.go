package storage

import (
	"time"
)

// FileInfo contains metadata about a stored object.
type FileInfo struct {
	// Path is the object's key within the store.
	Path string `json:"path"`
	// Size is the object's size in bytes.
	Size int64 `json:"size"`
	// LastModified is the time the object was last written.
	LastModified time.Time `json:"last_modified"`
	// ContentType is the object's MIME type.
	ContentType string `json:"content_type"`
	// Metadata holds backend-provided user metadata for the object.
	Metadata map[string]string `json:"metadata,omitempty"`
	// Checksum is the backend-provided content checksum, when available.
	Checksum string `json:"checksum,omitempty"`
}
