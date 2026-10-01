package connect

import (
	"testing"
	"time"

	connectrpc "connectrpc.com/connect"

	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
)

const testDomain = "gokit.test.v1.TestService"

func TestToConnectError_Nil(t *testing.T) {
	t.Parallel()
	if mustEncode(t, nil, testDomain) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestFromConnectError_Nil(t *testing.T) {
	t.Parallel()
	if mustDecode(t, nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestFromConnectError_NonConnectError(t *testing.T) {
	t.Parallel()
	appErr, err := DecodeError(apperrors.Internal(nil))
	if appErr != nil || err == nil {
		t.Fatalf("expected explicit non-transport decode failure, got %v, %v", appErr, err)
	}
}

func mustEncode(t *testing.T, err *apperrors.AppError, service string) *connectrpc.Error {
	t.Helper()
	result, encodeErr := ToConnectError(err, service)
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

// TestWireRoundTrip proves every vocabulary field survives AppError → Connect →
// AppError without loss, including codes that share one RPC status.
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

			cerr := mustEncode(t, original, testDomain)
			if cerr == nil {
				t.Fatal("ToConnectError returned nil")
			}
			if got, want := cerr.Code(), connectrpc.Code(apperrors.RPCCodeFor(original.Code)); got != want {
				t.Fatalf("connect code = %v, want %v", got, want)
			}

			got := mustDecode(t, cerr)
			if got == nil {
				t.Fatal("FromConnectError returned nil")
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

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	if IsRetryable(nil) {
		t.Error("nil is not retryable")
	}
	if IsRetryable(apperrors.Internal(nil)) {
		t.Error("a non-connect error is not retryable")
	}

	retry := mustEncode(t, apperrors.ServiceUnavailable("db"), testDomain)
	if !IsRetryable(retry) {
		t.Error("service unavailable should be retryable")
	}

	noRetry := mustEncode(t, apperrors.NotFound("user", "1"), testDomain)
	if IsRetryable(noRetry) {
		t.Error("not found should not be retryable")
	}
}
