//go:build unix

package fs

import (
	"os"
	"syscall"
)

// openAppendFile opens path for appending with atomic symlink rejection
// (O_NOFOLLOW) and non-blocking open (O_NONBLOCK) so a symlinked final
// component fails instead of being followed and a FIFO with no reader fails
// instead of blocking. O_NONBLOCK has no effect on writes to a regular file.
func openAppendFile(path string) (*os.File, error) {
	return os.OpenFile(path,
		os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
}
