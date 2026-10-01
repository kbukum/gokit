package errors

import (
	"context"
	stderrors "errors"
)

// Normalize preserves explicit application outcomes, classifies otherwise-untyped context failures, and hides unknown messages. The outward boundary owns cause logging.
func Normalize(err error) *AppError {
	if err == nil {
		return nil
	}

	if appErr, ok := AsAppError(err); ok {
		return appErr
	}

	if stderrors.Is(err, context.Canceled) {
		return Canceled("request").WithCause(err)
	}
	if stderrors.Is(err, context.DeadlineExceeded) {
		return Timeout("request").WithCause(err)
	}

	return Internal(err)
}
