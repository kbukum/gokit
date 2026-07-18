package fs

import (
	"errors"
	"net/http"
	"os"

	apperrors "github.com/kbukum/gokit/errors"
)

// osErrorCode classifies an error from an os filesystem call. A missing path maps to a typed not-found (404) so callers handling user-provided paths can react to it distinctly; any other failure maps to internal (500).
func osErrorCode(err error) (code apperrors.ErrorCode, status int) {
	if errors.Is(err, os.ErrNotExist) {
		return apperrors.ErrCodeNotFound, http.StatusNotFound
	}
	return apperrors.ErrCodeInternal, http.StatusInternalServerError
}
