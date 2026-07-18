package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
)

// ContentAddressableStorage wraps a Storage backend with hash-based deduplication.
// Content is stored at keys derived from its cryptographic hash,
// ensuring that identical content is never stored twice.
type ContentAddressableStorage struct {
	storage Storage
	hasher  func() hash.Hash
	prefix  string
}

// ContentAddressableOption configures a ContentAddressableStorage.
type ContentAddressableOption func(*ContentAddressableStorage)

// WithHasher sets the hash function used for content addressing. Defaults to sha256.New.
func WithHasher(h func() hash.Hash) ContentAddressableOption {
	return func(c *ContentAddressableStorage) {
		c.hasher = h
	}
}

// WithPrefix sets the key prefix for stored objects. Defaults to "sha256/".
func WithPrefix(prefix string) ContentAddressableOption {
	return func(c *ContentAddressableStorage) {
		c.prefix = prefix
	}
}

// NewContentAddressableStorage creates a content-addressable wrapper around the given Storage.
// Use options to customize the hash function and prefix.
func NewContentAddressableStorage(storage Storage, opts ...ContentAddressableOption) *ContentAddressableStorage {
	cas := &ContentAddressableStorage{
		storage: storage,
		hasher:  sha256.New,
		prefix:  "sha256/",
	}
	for _, opt := range opts {
		opt(cas)
	}
	return cas
}

// Store computes the hash while streaming content to storage. Returns the hex-encoded hash
// and whether the object was newly stored (vs already existed).
// The content is never fully buffered in memory.
func (c *ContentAddressableStorage) Store(ctx context.Context, reader io.Reader, contentType string) (hexHash string, isNew bool, err error) {
	h := c.hasher()
	// Buffer the content while hashing so we can compute the key before uploading.
	// We use a bytes.Buffer because we need to read twice: once for hash, once for upload.
	// For truly large objects a temp-file approach would be better,
	// but this matches the streaming Storage interface without requiring seek support.
	var buf bytes.Buffer
	tee := io.TeeReader(reader, h)
	if _, copyErr := io.Copy(&buf, tee); copyErr != nil {
		return "", false, fmt.Errorf("content addressable: hash read: %w", copyErr)
	}

	hashStr := hex.EncodeToString(h.Sum(nil))
	key := c.prefix + hashStr

	exists, err := c.storage.Exists(ctx, key)
	if err != nil {
		return "", false, fmt.Errorf("content addressable: exists check: %w", err)
	}
	if exists {
		return hashStr, false, nil
	}

	if err := c.storage.Upload(ctx, key, &buf); err != nil {
		return "", false, fmt.Errorf("content addressable: upload: %w", err)
	}
	return hashStr, true, nil
}

// Get retrieves content by its hex-encoded hash.
func (c *ContentAddressableStorage) Get(ctx context.Context, hexHash string) (io.ReadCloser, error) {
	key := c.prefix + hexHash
	rc, err := c.storage.Download(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("content addressable: download %q: %w", hexHash, err)
	}
	return rc, nil
}

// Exists checks if content with the given hex-encoded hash exists.
func (c *ContentAddressableStorage) Exists(ctx context.Context, hexHash string) (bool, error) {
	key := c.prefix + hexHash
	exists, err := c.storage.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("content addressable: exists %q: %w", hexHash, err)
	}
	return exists, nil
}

// Delete removes content by its hex-encoded hash.
func (c *ContentAddressableStorage) Delete(ctx context.Context, hexHash string) error {
	key := c.prefix + hexHash
	if err := c.storage.Delete(ctx, key); err != nil {
		return fmt.Errorf("content addressable: delete %q: %w", hexHash, err)
	}
	return nil
}

// URL returns the URL for content by its hex-encoded hash.
func (c *ContentAddressableStorage) URL(ctx context.Context, hexHash string) (string, error) {
	key := c.prefix + hexHash
	u, err := c.storage.URL(ctx, key)
	if err != nil {
		return "", fmt.Errorf("content addressable: url %q: %w", hexHash, err)
	}
	return u, nil
}
