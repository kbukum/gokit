package fs

import (
	"errors"
	"fmt"
	"io"
	"math"

	apperrors "github.com/kbukum/gokit/errors"
)

// Sentinel errors returned by the bounded reader for classifiable, policy-level rejections.
// They are plain sentinels (not AppError) so callers can match with errors.Is;
// raw IO failures (missing path, permission) return typed AppError.
var (
	// ErrFileTooLarge means a bounded read reached its byte limit.
	ErrFileTooLarge = errors.New("file exceeds size limit")
	// ErrNotRegularFile means the target is not a regular file.
	ErrNotRegularFile = errors.New("path is not a regular file")
)

// ReadFileLimit bounds a regular-file read, follows symlinks and reports descriptor/close failures. Unix opens are nonblocking before descriptor validation, so a FIFO cannot block opening. Filesystem operations are synchronous; this is not a universal filesystem cancellation guarantee.
// A negative maxBytes is rejected with an InvalidInput [apperrors.AppError].
// This operation neither confines paths nor rejects symlinks.
// A non-regular target yields [ErrNotRegularFile], an oversized file yields [ErrFileTooLarge],
// and other IO failures return a typed AppError.
func ReadFileLimit(path string, maxBytes int64) (data []byte, err error) {
	if maxBytes < 0 {
		return nil, apperrors.InvalidInput("maxBytes",
			fmt.Sprintf("maxBytes must be non-negative, got %d", maxBytes))
	}
	f, err := openReadFile(path)
	if err != nil {
		code := osErrorCode(err)
		return nil, apperrors.New(code,
			"failed to open file").WithCause(err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			err = errors.Join(err, apperrors.Internal(fmt.Errorf("close bounded file reader: %w", closeErr)))
			data = nil
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCodeInternal,
			"failed to inspect file",
		).WithCause(err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrNotRegularFile, path)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%w: %s (limit %d bytes)", ErrFileTooLarge, path, maxBytes)
	}
	// Read one byte past the limit to detect a file that grew after Stat,
	// guarding against overflow when maxBytes is at its maximum.
	probe := maxBytes
	if probe < math.MaxInt64 {
		probe++
	}
	data, err = io.ReadAll(io.LimitReader(f, probe))
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCodeInternal,
			"failed to read file",
		).WithCause(err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: %s (limit %d bytes)", ErrFileTooLarge, path, maxBytes)
	}
	return data, nil
}
