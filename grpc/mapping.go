package grpc

import (
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/protoadapt"
	"google.golang.org/protobuf/types/known/wrapperspb"

	apperrors "github.com/kbukum/gokit/errors"
)

// This file holds the canonical, lossless AppError <-> gRPC status mapping pair
// (the cross-kit parity surface, equivalent to rskit's app_error_to_status /
// status_to_app_error). It embeds the RFC 9457 ProblemDetail in the status
// details so a round trip preserves the error code, message, and extension
// members. The user-facing convenience helpers FromGRPC / ToGRPCStatus in
// errors.go remap to friendlier client messages and are a separate concern.

// ErrorCodeToGRPCCode maps a gokit ErrorCode to its canonical gRPC code.
func ErrorCodeToGRPCCode(code apperrors.ErrorCode) codes.Code {
	switch code {
	case apperrors.ErrCodeServiceUnavailable, apperrors.ErrCodeConnectionFailed:
		return codes.Unavailable
	case apperrors.ErrCodeTimeout:
		return codes.DeadlineExceeded
	case apperrors.ErrCodeRateLimited:
		return codes.ResourceExhausted
	case apperrors.ErrCodeNotFound:
		return codes.NotFound
	case apperrors.ErrCodeAlreadyExists:
		return codes.AlreadyExists
	case apperrors.ErrCodeConflict:
		return codes.Aborted
	case apperrors.ErrCodeInvalidInput, apperrors.ErrCodeMissingField, apperrors.ErrCodeInvalidFormat:
		return codes.InvalidArgument
	case apperrors.ErrCodeUnauthorized, apperrors.ErrCodeTokenExpired, apperrors.ErrCodeInvalidToken:
		return codes.Unauthenticated
	case apperrors.ErrCodeForbidden:
		return codes.PermissionDenied
	case apperrors.ErrCodeInternal, apperrors.ErrCodeDatabaseError, apperrors.ErrCodeExternalService:
		return codes.Internal
	default:
		return codes.Unknown
	}
}

// GRPCCodeToErrorCode maps a gRPC code to its canonical gokit ErrorCode.
func GRPCCodeToErrorCode(code codes.Code) apperrors.ErrorCode {
	switch code {
	case codes.Unavailable:
		return apperrors.ErrCodeServiceUnavailable
	case codes.DeadlineExceeded:
		return apperrors.ErrCodeTimeout
	case codes.ResourceExhausted:
		return apperrors.ErrCodeRateLimited
	case codes.NotFound:
		return apperrors.ErrCodeNotFound
	case codes.AlreadyExists:
		return apperrors.ErrCodeAlreadyExists
	case codes.Aborted, codes.FailedPrecondition:
		return apperrors.ErrCodeConflict
	case codes.InvalidArgument, codes.OutOfRange:
		return apperrors.ErrCodeInvalidInput
	case codes.Unauthenticated:
		return apperrors.ErrCodeUnauthorized
	case codes.PermissionDenied:
		return apperrors.ErrCodeForbidden
	case codes.Internal, codes.Unknown, codes.Unimplemented:
		return apperrors.ErrCodeInternal
	case codes.DataLoss:
		return apperrors.ErrCodeExternalService
	default:
		return apperrors.ErrCodeExternalService
	}
}

// AppErrorToStatus converts an AppError to a canonical gRPC status.
// The full RFC 9457 ProblemDetail is embedded in the status details so that
// StatusToAppError can reconstruct the error losslessly across the wire.
//
// This is the lossless internal mapping: the status message and embedded
// ProblemDetail (including the Details map) carry the original error verbatim.
// Do not use it at an untrusted external boundary where internal detail must
// not leak — use the ToGRPCStatus convenience helper, which remaps to a
// client-safe message, for those surfaces.
func AppErrorToStatus(appErr *apperrors.AppError) *status.Status {
	if appErr == nil {
		return nil
	}

	st := status.New(ErrorCodeToGRPCCode(appErr.Code), appErr.Message)

	payload, err := json.Marshal(appErr.ToProblemDetail())
	if err != nil {
		return st
	}
	detail := protoadapt.MessageV1Of(wrapperspb.Bytes(payload))
	if withDetails, derr := st.WithDetails(detail); derr == nil {
		return withDetails
	}
	return st
}

// StatusToAppError converts a gRPC status to an AppError.
// When the status carries an embedded ProblemDetail it is decoded for full
// fidelity; otherwise the error is reconstructed from the gRPC code and message.
func StatusToAppError(st *status.Status) *apperrors.AppError {
	if st == nil {
		return nil
	}

	if appErr := appErrorFromStatusDetails(st); appErr != nil {
		return appErr
	}

	code := GRPCCodeToErrorCode(st.Code())
	return &apperrors.AppError{
		Code:      code,
		Message:   st.Message(),
		Retryable: apperrors.IsRetryableCode(code),
	}
}

// appErrorFromStatusDetails recovers an AppError from an embedded ProblemDetail,
// or nil when the status carries no decodable ProblemDetail.
func appErrorFromStatusDetails(st *status.Status) *apperrors.AppError {
	for _, d := range st.Details() {
		bytesValue, ok := d.(*wrapperspb.BytesValue)
		if !ok {
			continue
		}
		var pd apperrors.ProblemDetail
		if err := json.Unmarshal(bytesValue.GetValue(), &pd); err != nil || pd.Code == "" {
			continue
		}
		return &apperrors.AppError{
			Code:       pd.Code,
			Message:    pd.Detail,
			Retryable:  pd.Retryable,
			HTTPStatus: pd.Status,
			Details:    pd.Details,
		}
	}
	return nil
}
