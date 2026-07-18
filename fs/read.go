package fs

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"

	apperrors "github.com/kbukum/gokit/errors"
)

// Sentinel errors returned by the bounded reader for classifiable, policy-level rejections. They are plain sentinels (not AppError) so callers can match with errors.Is; raw IO failures (missing path, permission) return typed AppError.
var (
	// ErrFileTooLarge means a bounded read reached its byte limit.
	ErrFileTooLarge = errors.New("file exceeds size limit")
	// ErrNotRegularFile means the target is not a regular file.
	ErrNotRegularFile = errors.New("path is not a regular file")
)

// ReadFileLimit reads at most maxBytes from a regular file, failing closed once the limit is exceeded so a caller never buffers an unbounded file. A negative maxBytes is rejected with an InvalidInput [apperrors.AppError]. It resolves symlinks like [os.Open]; callers that must reject symlinks verify the path first. A non-regular target yields [ErrNotRegularFile], an oversized file yields [ErrFileTooLarge], and other IO failures return a typed AppError.
func ReadFileLimit(path string, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 {
		return nil, apperrors.InvalidInput("maxBytes",
			fmt.Sprintf("maxBytes must be non-negative, got %d", maxBytes))
	}
	f, err := os.Open(path)
	if err != nil {
		code, status := osErrorCode(err)
		return nil, apperrors.New(code,
			fmt.Sprintf("failed to open '%s': %v", path, err), status).WithCause(err)
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to inspect '%s': %v", path, err),
			http.StatusInternalServerError).WithCause(err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrNotRegularFile, path)
	}
	if info.Size() > maxBytes {
		return nil, fmt.Errorf("%w: %s (limit %d bytes)", ErrFileTooLarge, path, maxBytes)
	}
	// Read one byte past the limit to detect a file that grew after Stat, guarding against overflow when maxBytes is at its maximum.
	probe := maxBytes
	if probe < math.MaxInt64 {
		probe++
	}
	data, err := io.ReadAll(io.LimitReader(f, probe))
	if err != nil {
		return nil, apperrors.New(apperrors.ErrCodeInternal,
			fmt.Sprintf("failed to read '%s': %v", path, err),
			http.StatusInternalServerError).WithCause(err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%w: %s (limit %d bytes)", ErrFileTooLarge, path, maxBytes)
	}
	return data, nil
}
