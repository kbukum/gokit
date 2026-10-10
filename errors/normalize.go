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
		if appErr == nil {
			return Internal(nil)
		}
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

// FromContext classifies a finished context: DeadlineExceeded becomes TIMEOUT and any other cancellation becomes CANCELED, with context.Cause as the cause. It returns nil while ctx is live.
func FromContext(ctx context.Context, operation string) *AppError {
	err := ctx.Err()
	if err == nil {
		return nil
	}
	if stderrors.Is(err, context.DeadlineExceeded) {
		return Timeout(operation).WithCause(context.Cause(ctx))
	}
	return Canceled(operation).WithCause(context.Cause(ctx))
}
