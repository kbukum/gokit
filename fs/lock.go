package fs

import (
	"context"
	"errors"
	"os"
	"runtime"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

const (
	fileLockWait = 5 * time.Second
	fileLockPoll = 10 * time.Millisecond
)

// WithFileLock serializes cooperating processes on a stable regular lock file created with mode 0600. The parent directory must be trusted; Windows inherits its ACLs rather than enforcing Unix permission bits. Acquisition respects ctx and waits at most five seconds. The operation must bound its own work. The lock is released even on operation failure or process exit. The lock file must not be removed or replaced: doing so would let another caller lock a different inode. All writers must use the same lock path.
func WithFileLock(ctx context.Context, path string, operation func() error) (err error) {
	if path == "" || operation == nil {
		return apperrors.InvalidInput("file lock", "A lock path and operation are required")
	}
	wait, cancel := context.WithTimeout(ctx, fileLockWait)
	defer cancel()
	if cause := wait.Err(); cause != nil {
		return apperrors.Internal(cause)
	}
	file, err := openAppendFile(path)
	if err != nil {
		return apperrors.Internal(err)
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return apperrors.Internal(err)
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return apperrors.InvalidInput("file lock", "Lock file must be regular and owner-only")
	}
	if cause := acquireFileLock(wait, file); cause != nil {
		return apperrors.Internal(cause)
	}
	defer func() { err = errors.Join(err, unlockFile(file)) }()
	if cause := wait.Err(); cause != nil {
		return apperrors.Internal(cause)
	}
	return operation()
}

func acquireFileLock(ctx context.Context, file *os.File) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		acquired, err := tryFileLock(file)
		if err != nil {
			return err
		}
		if acquired {
			return nil
		}
		timer := time.NewTimer(fileLockPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
