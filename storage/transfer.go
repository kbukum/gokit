// Cross-store helper: streaming Transfer between stores.
package storage

import (
	"context"
	"fmt"
)

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
