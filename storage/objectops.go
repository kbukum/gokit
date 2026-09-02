// Single-object helpers: Head, Copy, and Rename against any Storage.
package storage

import (
	"context"
	"fmt"
)

// Head returns metadata for a single object. Backends that implement
// [HeadProvider] answer directly; otherwise Head falls back to an exact-path
// match within List, so it works against any [Storage].
func Head(ctx context.Context, s Storage, path string) (FileInfo, error) {
	if s == nil {
		return FileInfo{}, fmt.Errorf("storage: Head requires a non-nil store")
	}
	if path == "" {
		return FileInfo{}, fmt.Errorf("storage: Head requires a non-empty path")
	}
	if hp, ok := s.(HeadProvider); ok {
		return hp.Head(ctx, path)
	}
	infos, err := s.List(ctx, path)
	if err != nil {
		return FileInfo{}, fmt.Errorf("storage: Head list %q: %w", path, err)
	}
	for _, info := range infos {
		if info.Path == path {
			return info, nil
		}
	}
	return FileInfo{}, NotFoundError(path)
}

// Copy duplicates an object within a single store, leaving the source in place.
// Backends that implement [CopyProvider] perform a server-side copy; otherwise
// Copy streams the object through [Transfer], so it works against any [Storage].
func Copy(ctx context.Context, s Storage, srcPath, dstPath string) error {
	if err := checkObjectOp("Copy", s, srcPath, dstPath); err != nil {
		return err
	}
	if cp, ok := s.(CopyProvider); ok {
		return cp.Copy(ctx, srcPath, dstPath)
	}
	return Transfer(ctx, s, srcPath, s, dstPath)
}

// Rename moves an object within a single store. Backends that implement
// [RenameProvider] perform a server-side move; otherwise Rename copies the
// object through [Copy] and deletes the source, so it works against any
// [Storage]. A failed delete leaves the destination in place and is reported.
func Rename(ctx context.Context, s Storage, srcPath, dstPath string) error {
	if err := checkObjectOp("Rename", s, srcPath, dstPath); err != nil {
		return err
	}
	if rp, ok := s.(RenameProvider); ok {
		return rp.Rename(ctx, srcPath, dstPath)
	}
	if err := Copy(ctx, s, srcPath, dstPath); err != nil {
		return fmt.Errorf("storage: Rename copy %q: %w", srcPath, err)
	}
	if err := s.Delete(ctx, srcPath); err != nil {
		return fmt.Errorf("storage: Rename delete %q: %w", srcPath, err)
	}
	return nil
}

// checkObjectOp validates the store and paths shared by the two-path helpers.
func checkObjectOp(op string, s Storage, srcPath, dstPath string) error {
	switch {
	case s == nil:
		return fmt.Errorf("storage: %s requires a non-nil store", op)
	case srcPath == "":
		return fmt.Errorf("storage: %s requires a non-empty source path", op)
	case dstPath == "":
		return fmt.Errorf("storage: %s requires a non-empty destination path", op)
	}
	return CheckDistinctPaths(op, srcPath, dstPath)
}
