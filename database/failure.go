package database

import (
	"context"
	"errors"

	apperrors "github.com/kbukum/gokit/errors"
)

type failure struct {
	cause error
}

// WrapFailure hides a driver error behind a safe classification. Driver diagnostics may contain credentials or SQL, so the message never formats the cause and callers must not log the unwrapped cause. The classification unwraps first, so [apperrors.Normalize] and every boundary built on it report DATABASE_FAILURE (or TIMEOUT/CANCELED for a context end) instead of an application error found inside the private cause; errors.Is still reaches the cause.
func WrapFailure(cause error) error { return failure{cause: cause} }

func (failure) Error() string      { return "DATABASE_FAILURE: Database operation failed" }
func (f failure) GoString() string { return f.Error() }

func (f failure) Unwrap() []error {
	classification := f.classification()
	if f.cause == nil {
		return []error{classification}
	}
	return []error{classification, f.cause}
}

// classification carries no cause, so boundaries that log a normalized error's cause cannot print driver text.
func (f failure) classification() *apperrors.AppError {
	switch {
	case errors.Is(f.cause, context.DeadlineExceeded):
		return apperrors.Timeout("database")
	case errors.Is(f.cause, context.Canceled):
		return apperrors.Canceled("database")
	default:
		return apperrors.New(apperrors.ErrCodeInternal, "Database operation failed").WithReason("DATABASE_FAILURE")
	}
}
