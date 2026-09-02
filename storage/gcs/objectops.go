package gcs

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	gcstorage "cloud.google.com/go/storage"

	"github.com/kbukum/gokit/storage"
)

// Head returns metadata for a single GCS object, including its metadata map and
// a content checksum derived from the object's MD5 or CRC32C attribute.
func (s *Storage) Head(ctx context.Context, path string) (storage.FileInfo, error) {
	fi, err := s.client.Head(ctx, path)
	if err != nil {
		return storage.FileInfo{}, fmt.Errorf("storage: gcs head: %w", err)
	}
	return fi, nil
}

// Copy duplicates an object within the bucket, leaving the source in place.
func (s *Storage) Copy(ctx context.Context, srcPath, dstPath string) error {
	if err := storage.CheckDistinctPaths("Copy", srcPath, dstPath); err != nil {
		return err
	}
	if err := s.client.Copy(ctx, srcPath, dstPath); err != nil {
		return fmt.Errorf("storage: gcs copy: %w", err)
	}
	return nil
}

// Rename moves an object within the bucket by copying then deleting the source.
func (s *Storage) Rename(ctx context.Context, srcPath, dstPath string) error {
	if err := storage.CheckDistinctPaths("Rename", srcPath, dstPath); err != nil {
		return err
	}
	if err := s.client.Copy(ctx, srcPath, dstPath); err != nil {
		return fmt.Errorf("storage: gcs rename copy: %w", err)
	}
	if err := s.client.Delete(ctx, srcPath); err != nil {
		return fmt.Errorf("storage: gcs rename delete: %w", err)
	}
	return nil
}

func (c *googleClient) Head(ctx context.Context, path string) (storage.FileInfo, error) {
	attrs, err := c.object(path).Attrs(ctx)
	if err != nil {
		if errors.Is(err, gcstorage.ErrObjectNotExist) {
			return storage.FileInfo{}, storage.NotFoundError(path)
		}
		return storage.FileInfo{}, err
	}
	fi := storage.FileInfo{
		Path:         path,
		Size:         attrs.Size,
		LastModified: attrs.Updated,
		ContentType:  attrs.ContentType,
		Checksum:     checksum(attrs),
	}
	if len(attrs.Metadata) > 0 {
		fi.Metadata = make(map[string]string, len(attrs.Metadata))
		for k, v := range attrs.Metadata {
			fi.Metadata[k] = v
		}
	}
	return fi, nil
}

func (c *googleClient) Copy(ctx context.Context, srcPath, dstPath string) error {
	src := c.object(srcPath)
	dst := c.object(dstPath)
	if _, err := dst.CopierFrom(src).Run(ctx); err != nil {
		if errors.Is(err, gcstorage.ErrObjectNotExist) {
			return storage.NotFoundError(srcPath)
		}
		return err
	}
	return nil
}

// checksum renders a GCS object's content hash as a hex string, preferring the
// MD5 digest and falling back to the CRC32C checksum.
func checksum(attrs *gcstorage.ObjectAttrs) string {
	if len(attrs.MD5) > 0 {
		return hex.EncodeToString(attrs.MD5)
	}
	if attrs.CRC32C != 0 {
		return fmt.Sprintf("%08x", attrs.CRC32C)
	}
	return ""
}
