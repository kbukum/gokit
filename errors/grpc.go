package errors

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// gRPC code mapping from ErrorCode to gRPC status codes.
var grpcCodeMap = map[ErrorCode]codes.Code{
	ErrCodeServiceUnavailable: codes.Unavailable,
	ErrCodeConnectionFailed:   codes.Unavailable,
	ErrCodeTimeout:            codes.DeadlineExceeded,
	ErrCodeRateLimited:        codes.ResourceExhausted,
	ErrCodeNotFound:           codes.NotFound,
	ErrCodeAlreadyExists:      codes.AlreadyExists,
	ErrCodeConflict:           codes.Aborted,
	ErrCodeInvalidInput:       codes.InvalidArgument,
	ErrCodeMissingField:       codes.InvalidArgument,
	ErrCodeInvalidFormat:      codes.InvalidArgument,
	ErrCodeUnauthorized:       codes.Unauthenticated,
	ErrCodeForbidden:          codes.PermissionDenied,
	ErrCodeTokenExpired:       codes.Unauthenticated,
	ErrCodeInvalidToken:       codes.Unauthenticated,
	ErrCodeInternal:           codes.Internal,
	ErrCodeDatabaseError:      codes.Internal,
	ErrCodeExternalService:    codes.Internal,
}

// reverse mapping from gRPC codes to ErrorCode.
var reverseGRPCMap = map[codes.Code]ErrorCode{
	codes.Unavailable:       ErrCodeServiceUnavailable,
	codes.DeadlineExceeded:  ErrCodeTimeout,
	codes.ResourceExhausted: ErrCodeRateLimited,
	codes.NotFound:          ErrCodeNotFound,
	codes.AlreadyExists:     ErrCodeAlreadyExists,
	codes.Aborted:           ErrCodeConflict,
	codes.InvalidArgument:   ErrCodeInvalidInput,
	codes.Unauthenticated:   ErrCodeUnauthorized,
	codes.PermissionDenied:  ErrCodeForbidden,
	codes.Internal:          ErrCodeInternal,
}

// ToGRPCStatus converts an AppError to a gRPC Status.
func (e *AppError) ToGRPCStatus() *status.Status {
	code, ok := grpcCodeMap[e.Code]
	if !ok {
		code = codes.Internal
	}
	return status.New(code, e.Message)
}

// FromGRPCStatus converts a gRPC Status to an AppError.
func FromGRPCStatus(s *status.Status) *AppError {
	code, ok := reverseGRPCMap[s.Code()]
	if !ok {
		code = ErrCodeInternal
	}
	return New(code, s.Message(), codeToHTTPStatus(code))
}

// codeToHTTPStatus maps ErrorCode to HTTP status for FromGRPCStatus.
func codeToHTTPStatus(code ErrorCode) int {
	switch code {
	case ErrCodeServiceUnavailable:
		return 503
	case ErrCodeConnectionFailed:
		return 502
	case ErrCodeTimeout:
		return 504
	case ErrCodeRateLimited:
		return 429
	case ErrCodeNotFound:
		return 404
	case ErrCodeAlreadyExists, ErrCodeConflict:
		return 409
	case ErrCodeInvalidInput, ErrCodeMissingField, ErrCodeInvalidFormat:
		return 422
	case ErrCodeUnauthorized, ErrCodeTokenExpired, ErrCodeInvalidToken:
		return 401
	case ErrCodeForbidden:
		return 403
	default:
		return 500
	}
}
