package errors

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"testing"
	"time"
)

// constructorCases pairs each helper constructor with its code so the test can
// assert the one-table invariant: a constructor never hardcodes a status or
// retryable verdict that disagrees with the central table.
func constructorCases() map[ErrorCode]*AppError {
	return map[ErrorCode]*AppError{
		ErrCodeServiceUnavailable: ServiceUnavailable("svc"),
		ErrCodeConnectionFailed:   ConnectionFailed("svc"),
		ErrCodeTimeout:            Timeout("op"),
		ErrCodeRateLimited:        RateLimited(),
		ErrCodeExternalService:    ExternalServiceError("svc", stderrors.New("x")),
		ErrCodeNotFound:           NotFound("user", "1"),
		ErrCodeAlreadyExists:      AlreadyExists("user"),
		ErrCodeConflict:           Conflict("busy"),
		ErrCodeInvalidInput:       InvalidInput("f", "bad"),
		ErrCodeMissingField:       MissingField("f"),
		ErrCodeInvalidFormat:      InvalidFormat("f", "email"),
		ErrCodeUnauthorized:       Unauthorized(""),
		ErrCodeForbidden:          Forbidden(""),
		ErrCodeTokenExpired:       TokenExpired(),
		ErrCodeInvalidToken:       InvalidToken(),
		ErrCodeInternal:           Internal(stderrors.New("x")),
		ErrCodeDatabaseError:      DatabaseError(stderrors.New("x")),
		ErrCodeCanceled:           Canceled("op"),
	}
}

func TestConstructorsAgreeWithTable(t *testing.T) {
	for code, err := range constructorCases() {
		if got, want := err.HTTPStatus(), HTTPStatusFor(code); got != want {
			t.Errorf("%s: HTTPStatus=%d, table says %d", code, got, want)
		}
		if got, want := err.Retryable, IsRetryableCode(code); got != want {
			t.Errorf("%s: Retryable=%v, table says %v", code, got, want)
		}
	}
}

func TestRPCCodeRoundTrip(t *testing.T) {
	cases := []struct {
		code ErrorCode
		rpc  RPCCode
	}{
		{ErrCodeNotFound, RPCCodeNotFound},
		{ErrCodeInvalidInput, RPCCodeInvalidArgument},
		{ErrCodeUnauthorized, RPCCodeUnauthenticated},
		{ErrCodeForbidden, RPCCodePermissionDenied},
		{ErrCodeConflict, RPCCodeFailedPrecondition},
		{ErrCodeTimeout, RPCCodeDeadlineExceeded},
		{ErrCodeRateLimited, RPCCodeResourceExhausted},
		{ErrCodeServiceUnavailable, RPCCodeUnavailable},
		{ErrCodeCanceled, RPCCodeCanceled},
		{ErrCodeInternal, RPCCodeInternal},
	}
	for _, c := range cases {
		if got := RPCCodeFor(c.code); got != c.rpc {
			t.Errorf("RPCCodeFor(%s)=%d, want %d", c.code, got, c.rpc)
		}
		if got := ErrorCodeForRPC(c.rpc); got != c.code {
			t.Errorf("ErrorCodeForRPC(%d)=%s, want %s", c.rpc, got, c.code)
		}
	}
	// Codes without a distinct reverse mapping collapse to INTERNAL.
	if got := ErrorCodeForRPC(RPCCodeAborted); got != ErrCodeInternal {
		t.Errorf("ErrorCodeForRPC(Aborted)=%s, want INTERNAL", got)
	}
}

func TestWithReasonAndTraceID(t *testing.T) {
	base := NotFound("user", "1")
	got := base.WithReason("GATE_BLOCKED").WithTraceID("trace-123")
	if got.Reason != "GATE_BLOCKED" || got.TraceID != "trace-123" {
		t.Fatalf("reason/trace not set: %+v", got)
	}
	if base.Reason != "" || base.TraceID != "" {
		t.Error("builders mutated the receiver")
	}
	if !stderrors.Is(got, base) {
		t.Error("enriched error no longer matches its origin")
	}
}

func TestWithViolationsCopies(t *testing.T) {
	vs := []Violation{{Field: "a", Reason: "REQUIRED", Message: "is required"}}
	got := InvalidInput("", "bad").WithViolations(vs...)
	if len(got.Violations) != 1 || got.Violations[0].Field != "a" {
		t.Fatalf("violations not set: %+v", got.Violations)
	}
	// Mutating the source slice must not affect the stored violations.
	vs[0].Field = "mutated"
	if got.Violations[0].Field != "a" {
		t.Error("WithViolations did not copy the slice")
	}
	// clone must deep-copy so enrichment does not alias.
	enriched := got.WithTraceID("t")
	enriched.Violations[0].Message = "changed"
	if got.Violations[0].Message != "is required" {
		t.Error("clone aliased the violations slice")
	}
	if got := got.WithViolations(); got.Violations != nil {
		t.Error("empty WithViolations should clear violations")
	}
}

func TestWithRetryAfter(t *testing.T) {
	got := Internal(nil).WithRetryAfter(2 * time.Second)
	if !got.Retryable {
		t.Error("WithRetryAfter should mark retryable")
	}
	if got.RetryAfter != 2*time.Second {
		t.Errorf("RetryAfter=%v, want 2s", got.RetryAfter)
	}
}

func TestNormalizeContextErrors(t *testing.T) {
	wrappedCancel := fmt.Errorf("db call: %w", context.Canceled)
	if got := Normalize(wrappedCancel); got.Code != ErrCodeCanceled {
		t.Errorf("wrapped cancel: code=%s, want CANCELED", got.Code)
	}
	wrappedDeadline := fmt.Errorf("db call: %w", context.DeadlineExceeded)
	if got := Normalize(wrappedDeadline); got.Code != ErrCodeTimeout {
		t.Errorf("wrapped deadline: code=%s, want TIMEOUT", got.Code)
	}
}

func TestNormalizePassthroughAndUnknown(t *testing.T) {
	app := NotFound("user", "1")
	if got := Normalize(fmt.Errorf("wrap: %w", app)); got.Code != ErrCodeNotFound {
		t.Errorf("AppError passthrough failed: %s", got.Code)
	}

	secret := stderrors.New("password=hunter2 at 10.0.0.1")
	got := Normalize(secret)
	if got.Code != ErrCodeInternal {
		t.Errorf("unknown error: code=%s, want INTERNAL", got.Code)
	}
	if got.Message == secret.Error() {
		t.Error("internal error exposed the raw cause as its message")
	}
	if !stderrors.Is(got, secret) {
		t.Error("internal error lost its cause for logging")
	}
	if Normalize(nil) != nil {
		t.Error("Normalize(nil) should be nil")
	}
}

func TestProblemDetailIncludesVocabulary(t *testing.T) {
	err := InvalidInput("", "bad").
		WithReason("BAD_SHAPE").
		WithViolations(Violation{Field: "email", Reason: "REQUIRED", Message: "is required"}).
		WithTraceID("trace-1")
	pd := err.ToProblemDetail()
	if pd.Reason != "BAD_SHAPE" || pd.TraceID != "trace-1" {
		t.Errorf("problem+json missing reason/trace: %+v", pd)
	}
	if len(pd.Violations) != 1 || pd.Violations[0].Reason != "REQUIRED" {
		t.Errorf("problem+json missing violations: %+v", pd.Violations)
	}
	if pd.RetryAfterSeconds != 0 {
		t.Error("non-retryable error should not carry retryAfter")
	}

	retry := RateLimited().WithRetryAfter(3 * time.Second)
	rpd := retry.ToProblemDetail()
	if rpd.RetryAfterSeconds != 3 {
		t.Errorf("retryAfter=%v, want 3", rpd.RetryAfterSeconds)
	}

	// retryAfter is omitted from JSON when zero.
	raw, err2 := json.Marshal(InvalidInput("", "bad").ToProblemDetail())
	if err2 != nil {
		t.Fatal(err2)
	}
	if contains(string(raw), "retryAfter") {
		t.Errorf("zero retryAfter should be omitted: %s", raw)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
