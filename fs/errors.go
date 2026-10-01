package fs

import (
	"errors"
	"os"

	apperrors "github.com/kbukum/gokit/errors"
)

// osErrorCode classifies an error from an os filesystem call.
// A missing path maps to a typed not-found (404)
// so callers handling user-provided paths can react to it distinctly;
// any other failure maps to internal (500).
func osErrorCode(err error) apperrors.ErrorCode {
	if errors.Is(err, os.ErrNotExist) {
		return apperrors.ErrCodeNotFound
	}
	return apperrors.ErrCodeInternal
}
