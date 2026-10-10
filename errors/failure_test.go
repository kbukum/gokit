package errors

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestNormalizeTypedNilApplicationFailure(t *testing.T) {
	t.Parallel()
	var missing *AppError
	for _, err := range []error{missing, fmt.Errorf("wrapped: %w", missing)} {
		got := Normalize(err)
		if got == nil || got.Code != ErrCodeInternal {
			t.Fatalf("typed-nil outcome must fail closed: %#v", got)
		}
	}
}

func TestFailureJSONVocabulary(t *testing.T) {
	t.Parallel()
	err := New(ErrCodeTimeout, "safe").WithRetryAfter(1500 * time.Millisecond).WithCause(context.Canceled)
	failure := err.ToFailure()
	data, marshalErr := json.Marshal(failure)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if !strings.Contains(string(data), `"retryAfter":1.5`) || !strings.Contains(string(data), `"code":"TIMEOUT"`) || !strings.Contains(string(data), `"message":"safe"`) {
		t.Fatalf("failure: %s", data)
	}
}

func TestFromContextClassifiesDeadlineAndCancellation(t *testing.T) {
	if FromContext(context.Background(), "op") != nil {
		t.Fatal("live context classified")
	}
	cause := stderrors.New("shutdown")
	canceled, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	if got := FromContext(canceled, "op"); got.Code != ErrCodeCanceled || !stderrors.Is(got, cause) {
		t.Fatalf("cancellation: %v", got)
	}
	expired, stop := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer stop()
	if got := FromContext(expired, "op"); got.Code != ErrCodeTimeout || !stderrors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", got)
	}
}
