package messaging

import apperrors "github.com/kbukum/gokit/errors"

// ErrorTranslator converts broker-specific errors to AppError. Each broker implementation maps its native errors to appropriate error codes, HTTP statuses, and retryable flags.
type ErrorTranslator interface {
	Translate(err error, topic string) *apperrors.AppError
}
