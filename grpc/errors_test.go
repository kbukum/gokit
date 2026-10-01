package grpc

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
)

const testDomain = "gokit.test.v1.TestService"

func TestAppErrorToStatus_Nil(t *testing.T) {
	t.Parallel()
	if mustEncode(t, nil, testDomain) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestStatusToAppError_Nil(t *testing.T) {
	t.Parallel()
	if mustDecode(t, nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestStatusToAppError_NonStatusError(t *testing.T) {
	t.Parallel()
	appErr, err := DecodeError(errors.New("some random failure"))
	if appErr != nil || err == nil {
		t.Fatalf("expected explicit non-transport decode failure, got %v, %v", appErr, err)
	}
}

// TestWireRoundTrip proves every vocabulary field survives AppError → gRPC status
// → AppError without loss, including codes that share one RPC status.
func TestWireRoundTrip(t *testing.T) {
	t.Parallel()

	cases := map[string]*apperrors.AppError{
		"not found with reason and trace": apperrors.NotFound("user", "123").
			WithReason("USER_GONE").WithTraceID("trace-abc"),
		"validation with violations": apperrors.Validation("request failed validation").
			WithViolations(
				apperrors.Violation{Field: "email", Reason: "REQUIRED", Message: "is required"},
				apperrors.Violation{Field: "age", Reason: "OUT_OF_RANGE", Message: "must be >= 18"},
			).WithReason("FORM_INVALID"),
		"rate limited retry with delay": apperrors.RateLimited().
			WithRetryAfter(1500 * time.Millisecond),
		"service unavailable retry no delay": apperrors.ServiceUnavailable("db"),
		"missing field keeps exact code":     apperrors.MissingField("name"),
		"invalid format keeps exact code":    apperrors.InvalidFormat("date", "RFC3339"),
		"database error keeps exact code":    apperrors.DatabaseError(nil),
		"external service keeps exact code":  apperrors.ExternalServiceError("stripe", nil),
		"token expired keeps exact code":     apperrors.TokenExpired(),
		"invalid token keeps exact code":     apperrors.InvalidToken(),
		"conflict keeps exact code":          apperrors.Conflict("version mismatch"),
		"already exists keeps exact code":    apperrors.AlreadyExists("user"),
		"forbidden keeps exact code":         apperrors.Forbidden("no access"),
		"canceled keeps exact code":          apperrors.Canceled("request"),
		"timeout keeps exact code":           apperrors.Timeout("request"),
		"internal keeps safe message":        apperrors.Internal(nil),
		"retryable override survives wire":   apperrors.Internal(nil).WithRetryAfter(0),
	}

	for name, original := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			st := mustEncode(t, original, testDomain)
			if st == nil {
				t.Fatal("AppErrorToStatus returned nil")
			}
			if got, want := st.Code(), codes.Code(apperrors.RPCCodeFor(original.Code)); got != want {
				t.Fatalf("grpc code = %v, want %v", got, want)
			}

			got := mustDecode(t, st.Err())
			if got == nil {
				t.Fatal("StatusToAppError returned nil")
			}
			if got.Code != original.Code {
				t.Errorf("code = %q, want %q", got.Code, original.Code)
			}
			if got.Reason != original.Reason {
				t.Errorf("reason = %q, want %q", got.Reason, original.Reason)
			}
			if got.Message != original.Message {
				t.Errorf("message = %q, want %q", got.Message, original.Message)
			}
			if got.TraceID != original.TraceID {
				t.Errorf("traceId = %q, want %q", got.TraceID, original.TraceID)
			}
			if got.Retryable != original.Retryable {
				t.Errorf("retryable = %v, want %v", got.Retryable, original.Retryable)
			}
			if got.RetryAfter != original.RetryAfter {
				t.Errorf("retryAfter = %v, want %v", got.RetryAfter, original.RetryAfter)
			}
			assertViolationsEqual(t, got.Violations, original.Violations)
		})
	}
}

func assertViolationsEqual(t *testing.T, got, want []apperrors.Violation) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("violations len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("violation[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestStatusToAppError_PlainStatusCodes proves a status without gokit details
// (for example one produced by a third-party gRPC server) still decodes to a
// sensible AppError using only the one code table.
func TestStatusToAppError_PlainStatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code      codes.Code
		wantCode  apperrors.ErrorCode
		wantRetry bool
	}{
		{codes.NotFound, apperrors.ErrCodeNotFound, false},
		{codes.AlreadyExists, apperrors.ErrCodeAlreadyExists, false},
		{codes.InvalidArgument, apperrors.ErrCodeInvalidInput, false},
		{codes.Unauthenticated, apperrors.ErrCodeUnauthorized, false},
		{codes.PermissionDenied, apperrors.ErrCodeForbidden, false},
		{codes.FailedPrecondition, apperrors.ErrCodeConflict, false},
		{codes.DeadlineExceeded, apperrors.ErrCodeTimeout, true},
		{codes.ResourceExhausted, apperrors.ErrCodeRateLimited, true},
		{codes.Unavailable, apperrors.ErrCodeServiceUnavailable, true},
		{codes.Internal, apperrors.ErrCodeInternal, false},
	}

	for _, tc := range tests {
		t.Run(tc.code.String(), func(t *testing.T) {
			t.Parallel()
			appErr := mustDecode(t, status.Error(tc.code, "boom"))
			if appErr.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", appErr.Code, tc.wantCode)
			}
			if appErr.HTTPStatus() != apperrors.HTTPStatusFor(tc.wantCode) {
				t.Errorf("httpStatus = %d, want %d", appErr.HTTPStatus(), apperrors.HTTPStatusFor(tc.wantCode))
			}
			if appErr.Retryable != tc.wantRetry {
				t.Errorf("retryable = %v, want %v", appErr.Retryable, tc.wantRetry)
			}
			if appErr.Unwrap() == nil {
				t.Error("cause should be set for a status error")
			}
		})
	}
}

func TestAppErrorToStatus_MessagePreserved(t *testing.T) {
	t.Parallel()
	st := mustEncode(t, apperrors.NotFound("user", "1"), testDomain)
	if st.Message() == "" {
		t.Fatal("expected a non-empty status message")
	}
	if st.Code() != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", st.Code())
	}
}

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	if IsRetryable(nil) {
		t.Error("nil is not retryable")
	}
	if IsRetryable(errors.New("plain error")) {
		t.Error("a non-status error is not retryable")
	}

	retry := mustEncode(t, apperrors.ServiceUnavailable("db"), testDomain).Err()
	if !IsRetryable(retry) {
		t.Error("service unavailable should be retryable")
	}

	// ExternalService is retryable but shares codes.Internal; the exact code in
	// ErrorInfo metadata must recover retryability.
	external := mustEncode(t, apperrors.ExternalServiceError("stripe", nil).WithRetryable(true), testDomain).Err()
	if !IsRetryable(external) {
		t.Error("external service error should be retryable via exact code")
	}

	noRetry := mustEncode(t, apperrors.NotFound("user", "1"), testDomain).Err()
	if IsRetryable(noRetry) {
		t.Error("not found should not be retryable")
	}
}

func mustEncode(t *testing.T, err *apperrors.AppError, service string) *status.Status {
	t.Helper()
	result, encodeErr := AppErrorToStatus(err, service)
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	return result
}

func mustDecode(t *testing.T, err error) *errorrpc.Error {
	t.Helper()
	result, decodeErr := DecodeError(err)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	return result
}
