package errors

import (
	"context"
	"encoding/json"
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
