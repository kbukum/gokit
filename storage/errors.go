// Portable error identities shared across storage backends.
package storage

import (
	"errors"
	"fmt"
)

// ErrNotFound is the portable identity for a missing object. Every backend wraps
// it (via [NotFoundError]) so callers can test for a missing object with
// errors.Is regardless of the underlying store.
var ErrNotFound = errors.New("storage: object not found")

// ErrSameObject reports a Copy or Rename whose source and destination keys are
// identical. Such a request cannot duplicate or move an object and, for backends
// that copy in place or copy-then-delete, would destroy it, so the package-level
// helpers and every adapter reject it before touching the store.
var ErrSameObject = errors.New("storage: source and destination path are the same")

// NotFoundError wraps [ErrNotFound] with the missing object's path.
func NotFoundError(path string) error {
	return fmt.Errorf("%w: %s", ErrNotFound, path)
}

// CheckDistinctPaths returns an [ErrSameObject] error when srcPath equals
// dstPath. Copy and Rename implementations call it at entry so a same-key request
// can never reach a destructive backend operation.
func CheckDistinctPaths(op, srcPath, dstPath string) error {
	if srcPath == dstPath {
		return fmt.Errorf("storage: %s source and destination path %q are the same: %w", op, srcPath, ErrSameObject)
	}
	return nil
}
