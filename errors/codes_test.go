package errors

import (
	"testing"

	"google.golang.org/grpc/codes"
)

func TestErrorCode_GRPCCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		code     ErrorCode
		expected codes.Code
	}{
		{"NotFound", ErrCodeNotFound, codes.NotFound},
		{"AlreadyExists", ErrCodeAlreadyExists, codes.AlreadyExists},
		{"InvalidInput", ErrCodeInvalidInput, codes.InvalidArgument},
		{"MissingField", ErrCodeMissingField, codes.InvalidArgument},
		{"InvalidFormat", ErrCodeInvalidFormat, codes.InvalidArgument},
		{"Unauthorized", ErrCodeUnauthorized, codes.Unauthenticated},
		{"TokenExpired", ErrCodeTokenExpired, codes.Unauthenticated},
		{"InvalidToken", ErrCodeInvalidToken, codes.Unauthenticated},
		{"Forbidden", ErrCodeForbidden, codes.PermissionDenied},
		{"Conflict", ErrCodeConflict, codes.Aborted},
		{"Timeout", ErrCodeTimeout, codes.DeadlineExceeded},
		{"RateLimited", ErrCodeRateLimited, codes.ResourceExhausted},
		{"ServiceUnavailable", ErrCodeServiceUnavailable, codes.Unavailable},
		{"ConnectionFailed", ErrCodeConnectionFailed, codes.Unavailable},
		{"Internal", ErrCodeInternal, codes.Internal},
		{"DatabaseError", ErrCodeDatabaseError, codes.Internal},
		{"ExternalService", ErrCodeExternalService, codes.Internal},
		{"Canceled", ErrCodeCanceled, codes.Canceled},
		{"Unknown code defaults to Internal", ErrorCode("UNKNOWN"), codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.code.GRPCCode(); got != tt.expected {
				t.Errorf("ErrorCode(%q).GRPCCode() = %v, want %v", tt.code, got, tt.expected)
			}
		})
	}
}
