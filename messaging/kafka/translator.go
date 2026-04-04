package kafka

import (
	"net/http"

	apperrors "github.com/kbukum/gokit/errors"
)

// FromKafka converts a Kafka error to an AppError.
// It translates Kafka-specific errors to user-friendly messages.
func FromKafka(err error, topic string) *apperrors.AppError {
	if err == nil {
		return nil
	}

	// Connection errors
	if IsConnectionError(err) {
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeServiceUnavailable,
			Message:    "Message queue is temporarily unavailable. Your request has been noted.",
			HTTPStatus: http.StatusServiceUnavailable,
			Retryable:  true,
			Details:    map[string]interface{}{"topic": topic},
		}).WithCause(err)
	}

	// Non-retryable errors (message too large, invalid topic, etc.)
	if IsNonRetryableError(err) {
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeInvalidInput,
			Message:    "Unable to process the message. Please check your input.",
			HTTPStatus: http.StatusBadRequest,
			Retryable:  false,
		}).WithCause(err)
	}

	// Retryable errors (transient failures)
	if IsRetryableError(err) {
		return (&apperrors.AppError{
			Code:       apperrors.ErrCodeExternalService,
			Message:    "Temporary processing error. Please try again.",
			HTTPStatus: http.StatusServiceUnavailable,
			Retryable:  true,
		}).WithCause(err)
	}

	// Default: internal error
	return apperrors.Internal(err)
}
