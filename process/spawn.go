package process

import (
	stderrors "errors"
	"fmt"
	"os"
	"os/exec"

	goerrors "github.com/kbukum/gokit/errors"
)

// SpawnError converts a subprocess spawn failure into a typed AppError.
//
// The OS detail is kept as the visible "<context>: <error>" message, while the error
// code is classified from the underlying failure: a missing executable becomes a
// NotFound error, a permission-denied failure becomes Forbidden, and anything else
// becomes Internal. Callers can then tell "not installed" apart from other spawn
// failures instead of seeing every failure collapse into a generic error. The original
// error is preserved as the cause so the underlying chain survives errors.Is/As.
func SpawnError(context string, err error) error {
	message := fmt.Sprintf("%s: %v", context, err)

	var appErr *goerrors.AppError
	switch {
	case stderrors.Is(err, exec.ErrNotFound) || os.IsNotExist(err):
		appErr = goerrors.NotFound("executable", "")
	case stderrors.Is(err, os.ErrPermission):
		appErr = goerrors.Forbidden("")
	default:
		appErr = goerrors.Internal(err)
	}

	appErr.Message = message
	return appErr.WithCause(err)
}
