//go:build !unix

package fs

import "os"

// applyArchiveMode is a no-op on platforms without Unix permission bits.
func applyArchiveMode(_, _ string, _ os.FileMode) error {
	return nil
}
