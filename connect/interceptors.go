package connect

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/logger"
)

// ---------------------------------------------------------------------------
// Logging
// ---------------------------------------------------------------------------

// LoggingInterceptor returns a Connect interceptor that logs every RPC call
// with procedure name, duration, and outcome.
func LoggingInterceptor(log *logger.Logger) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			start := time.Now()
			procedure := req.Spec().Procedure

			log.WithContext(ctx).Debug("RPC call started", map[string]interface{}{
				"procedure": procedure,
			})

			resp, err := next(ctx, req)
			duration := time.Since(start)

			fields := map[string]interface{}{
				"procedure":   procedure,
				"duration_ms": duration.Milliseconds(),
			}

			if err != nil {
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					fields["code"] = connectErr.Code().String()
				}
				fields["error"] = err.Error()
				log.WithContext(ctx).Error("RPC call failed", fields)
			} else {
				fields["code"] = "ok"
				log.WithContext(ctx).Debug("RPC call completed", fields)
			}

			return resp, err
		}
	}
}

// ---------------------------------------------------------------------------
// Error mapping  (AppError ↔ Connect)
// ---------------------------------------------------------------------------

// ErrorInterceptor returns a Connect interceptor that converts AppError
// instances returned by handlers into properly coded Connect errors.
func ErrorInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			resp, err := next(ctx, req)
			if err == nil {
				return resp, nil
			}

			// Already a Connect error — leave it alone.
			var connectErr *connect.Error
			if errors.As(err, &connectErr) {
				return resp, err
			}

			// Convert AppError → Connect error.
			var appErr *apperrors.AppError
			if errors.As(err, &appErr) {
				return resp, ToConnectError(appErr)
			}

			return resp, err
		}
	}
}

// ToConnectError converts an AppError to a *connect.Error with the
// appropriate Connect status code.
func ToConnectError(appErr *apperrors.AppError) *connect.Error {
	if appErr == nil {
		return nil
	}
	code := appErrorCodeToConnect(appErr.Code)
	return connect.NewError(code, errors.New(appErr.Message))
}

// FromConnectError converts a Connect error to an AppError.
func FromConnectError(err error) *apperrors.AppError {
	if err == nil {
		return nil
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		return apperrors.Internal(err)
	}

	return connectCodeToAppError(connectErr.Code(), connectErr.Message())
}

func appErrorCodeToConnect(code apperrors.ErrorCode) connect.Code {
	switch code {
	case apperrors.ErrCodeNotFound:
		return connect.CodeNotFound
	case apperrors.ErrCodeAlreadyExists:
		return connect.CodeAlreadyExists
	case apperrors.ErrCodeInvalidInput, apperrors.ErrCodeMissingField, apperrors.ErrCodeInvalidFormat:
		return connect.CodeInvalidArgument
	case apperrors.ErrCodeUnauthorized, apperrors.ErrCodeTokenExpired, apperrors.ErrCodeInvalidToken:
		return connect.CodeUnauthenticated
	case apperrors.ErrCodeForbidden:
		return connect.CodePermissionDenied
	case apperrors.ErrCodeConflict:
		return connect.CodeFailedPrecondition
	case apperrors.ErrCodeTimeout:
		return connect.CodeDeadlineExceeded
	case apperrors.ErrCodeRateLimited:
		return connect.CodeResourceExhausted
	case apperrors.ErrCodeServiceUnavailable, apperrors.ErrCodeConnectionFailed:
		return connect.CodeUnavailable
	case apperrors.ErrCodeDatabaseError, apperrors.ErrCodeExternalService, apperrors.ErrCodeInternal:
		return connect.CodeInternal
	default:
		return connect.CodeInternal
	}
}

func connectCodeToAppError(code connect.Code, msg string) *apperrors.AppError {
	switch code {
	case connect.CodeNotFound:
		return apperrors.NotFound("resource", "")
	case connect.CodeAlreadyExists:
		return apperrors.AlreadyExists("resource")
	case connect.CodeInvalidArgument:
		return apperrors.InvalidInput("", sanitizeMessage(msg))
	case connect.CodeUnauthenticated:
		return apperrors.Unauthorized(sanitizeMessage(msg))
	case connect.CodePermissionDenied:
		return apperrors.Forbidden(sanitizeMessage(msg))
	case connect.CodeFailedPrecondition:
		return apperrors.Conflict(sanitizeMessage(msg))
	case connect.CodeDeadlineExceeded:
		return apperrors.Timeout("rpc")
	case connect.CodeResourceExhausted:
		return apperrors.RateLimited()
	case connect.CodeUnavailable:
		return apperrors.ServiceUnavailable("service")
	case connect.CodeCanceled:
		return apperrors.Internal(errors.New("request canceled"))
	default:
		return apperrors.Internal(errors.New(msg))
	}
}

func sanitizeMessage(msg string) string {
	if msg == "" {
		return "An error occurred."
	}
	return msg
}

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

// AuthInterceptor returns a Connect interceptor that validates the
// Authorization header using the provided validateToken function.
// The token is extracted by stripping the "Bearer " prefix.
func AuthInterceptor(validateToken func(ctx context.Context, token string) error) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			header := req.Header().Get("Authorization")
			if header == "" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing authorization header"))
			}

			token := strings.TrimPrefix(header, "Bearer ")
			if token == header {
				// No "Bearer " prefix found.
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("invalid authorization scheme"))
			}

			if err := validateToken(ctx, token); err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}

			return next(ctx, req)
		}
	}
}
