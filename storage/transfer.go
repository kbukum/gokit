// Cross-store helpers: single-object Stat and streaming Transfer.
package storage

import (
	"context"
	"fmt"
)

// StatProvider is optionally implemented by storage backends that can return
// metadata for a single object more efficiently than scanning a listing.
type StatProvider interface {
	// Stat returns metadata for the object at path.
	Stat(ctx context.Context, path string) (FileInfo, error)
}

// Stat returns metadata for a single object. Backends that implement
// [StatProvider] answer directly; otherwise Stat falls back to an exact-path
// match within List, so it works against any [Storage].
func Stat(ctx context.Context, s Storage, path string) (FileInfo, error) {
	if s == nil {
		return FileInfo{}, fmt.Errorf("storage: Stat requires a non-nil store")
	}
	if path == "" {
		return FileInfo{}, fmt.Errorf("storage: Stat requires a non-empty path")
	}
	if sp, ok := s.(StatProvider); ok {
		return sp.Stat(ctx, path)
	}
	infos, err := s.List(ctx, path)
	if err != nil {
		return FileInfo{}, fmt.Errorf("storage: Stat list %q: %w", path, err)
	}
	for _, info := range infos {
		if info.Path == path {
			return info, nil
		}
	}
	return FileInfo{}, fmt.Errorf("storage: object not found: %s", path)
}

// Transfer streams the object at srcPath in src to dstPath in dst. It downloads
// and uploads without buffering the whole object in memory, so it works across
// backends of any size. The source object is left in place.
func Transfer(ctx context.Context, src Storage, srcPath string, dst Storage, dstPath string) error {
	switch {
	case src == nil:
		return fmt.Errorf("storage: Transfer requires a non-nil source store")
	case dst == nil:
		return fmt.Errorf("storage: Transfer requires a non-nil destination store")
	case srcPath == "":
		return fmt.Errorf("storage: Transfer requires a non-empty source path")
	case dstPath == "":
		return fmt.Errorf("storage: Transfer requires a non-empty destination path")
	}

	reader, err := src.Download(ctx, srcPath)
	if err != nil {
		return fmt.Errorf("storage: Transfer download %q: %w", srcPath, err)
	}
	defer func() { _ = reader.Close() }()

	if err := dst.Upload(ctx, dstPath, reader); err != nil {
		return fmt.Errorf("storage: Transfer upload %q: %w", dstPath, err)
	}
	return nil
}
