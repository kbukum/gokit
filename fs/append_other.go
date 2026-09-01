//go:build !unix

package fs

import "os"

// openAppendFile opens path for appending. Platforms without O_NOFOLLOW fall
// back to a pre-open Lstat check so a symlinked target is rejected rather than
// followed; OpenAppend still verifies the opened descriptor is a regular file.
func openAppendFile(path string) (*os.File, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrInvalid}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}
