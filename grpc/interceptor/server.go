package interceptor

import (
	"context"
	"errors"
	"path"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logging"
)

// UnaryServerLoggingInterceptor returns a unary server interceptor that logs each incoming RPC with method,
// duration, and status code.
func UnaryServerLoggingInterceptor(log *logging.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()
		svc := path.Dir(info.FullMethod)[1:]
		method := path.Base(info.FullMethod)

		log.DebugCtx(ctx, "gRPC request started", map[string]any{
			"service": svc,
			"method":  method,
		})

		resp, err := handler(ctx, req)
		duration := time.Since(start)

		fields := map[string]any{
			"service":     svc,
			"method":      method,
			"duration_ms": duration.Milliseconds(),
		}

		if err != nil {
			st := status.Convert(err)
			fields["status"] = st.Code().String()
			fields["error"] = st.Message()
			log.ErrorCtx(ctx, "gRPC request failed", fields)
		} else {
			fields["status"] = "OK"
			log.DebugCtx(ctx, "gRPC request completed", fields)
		}

		return resp, err
	}
}

// UnaryServerErrorInterceptor returns a unary server interceptor that converts AppError values returned by handlers into proper gRPC status errors,
// ensuring clients receive structured codes instead of codes.Unknown.
func UnaryServerErrorInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		var appErr *apperrors.AppError
		if errors.As(err, &appErr) {
			return nil, appErrorToGRPCStatus(appErr)
		}

		return resp, err
	}
}

// appErrorToGRPCStatus maps an AppError to a gRPC status error.
// Mirrors the inverse mapping in errors.go (FromGRPC) without creating a cross-package import cycle between interceptor ↔ root grpc package.
func appErrorToGRPCStatus(appErr *apperrors.AppError) error {
	var code codes.Code
	switch appErr.Code {
	case apperrors.ErrCodeNotFound:
		code = codes.NotFound
	case apperrors.ErrCodeAlreadyExists:
		code = codes.AlreadyExists
	case apperrors.ErrCodeInvalidInput, apperrors.ErrCodeMissingField, apperrors.ErrCodeInvalidFormat:
		code = codes.InvalidArgument
	case apperrors.ErrCodeUnauthorized, apperrors.ErrCodeTokenExpired, apperrors.ErrCodeInvalidToken:
		code = codes.Unauthenticated
	case apperrors.ErrCodeForbidden:
		code = codes.PermissionDenied
	case apperrors.ErrCodeConflict:
		code = codes.FailedPrecondition
	case apperrors.ErrCodeTimeout:
		code = codes.DeadlineExceeded
	case apperrors.ErrCodeRateLimited:
		code = codes.ResourceExhausted
	case apperrors.ErrCodeServiceUnavailable, apperrors.ErrCodeConnectionFailed:
		code = codes.Unavailable
	case apperrors.ErrCodeDatabaseError, apperrors.ErrCodeExternalService, apperrors.ErrCodeInternal:
		code = codes.Internal
	default:
		code = codes.Internal
	}
	return status.Error(code, appErr.Message)
}
