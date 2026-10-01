package rpc

import (
	"fmt"
	"time"

	apperrors "github.com/kbukum/gokit/errors"
)

// Error is a decoded remote failure. Its message is remote input, not an approved public AppError message. RPCCode preserves protocol identity even without a recognized application category.
type Error struct {
	Code       apperrors.ErrorCode
	RPCCode    apperrors.RPCCode
	Reason     string
	Message    string
	Violations []apperrors.Violation
	Retryable  bool
	RetryAfter time.Duration
	TraceID    string
	cause      error
}

func (e *Error) Error() string { return fmt.Sprintf("remote RPC %d: %s", e.RPCCode, e.Message) }

// Unwrap preserves the original transport error, never a trusted application error.
func (e *Error) Unwrap() error { return e.cause }

// HTTPStatus returns the REST recommendation for the decoded application category.
func (e *Error) HTTPStatus() int { return apperrors.HTTPStatusFor(e.Code) }

// DecodeError reports malformed or contradictory recognized wire details.
type DecodeError struct {
	Detail string
	Cause  error
}

func (e *DecodeError) Error() string { return "decode RPC error: " + e.Detail }
func (e *DecodeError) Unwrap() error { return e.Cause }
