package fs

import (
	"fmt"
	"os"

	apperrors "github.com/kbukum/gokit/errors"
)

// OpenAppend opens path for appending, creating it with owner-only 0600
// permissions when absent, and rejects symlink, FIFO, device, and directory
// targets so a caller cannot be redirected to another file or blocked opening a
// pipe. Where the platform supports it, symlink rejection and non-blocking open
// are atomic (O_NOFOLLOW/O_NONBLOCK); other platforms fall back to a pre-open
// symlink check. A non-regular target yields [ErrNotRegularFile]; other IO
// failures return a typed [apperrors.AppError]. The caller owns closing the
// returned file.
func OpenAppend(path string) (*os.File, error) {
	f, err := openAppendFile(path)
	if err != nil {
		code := osErrorCode(err)
		return nil, apperrors.New(code,
			"failed to open file").WithCause(err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, apperrors.New(apperrors.ErrCodeInternal,
			"failed to inspect file",
		).WithCause(err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("%w: %s", ErrNotRegularFile, path)
	}
	return f, nil
}
