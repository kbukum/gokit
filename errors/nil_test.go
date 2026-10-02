package errors

import (
	"context"
	stderrors "errors"
	"fmt"
	"testing"
)

func TestNormalizedNilSupportsErrorTraversal(t *testing.T) {
	t.Parallel()
	var leaf *AppError
	for _, input := range []error{leaf, fmt.Errorf("wrapped: %w", leaf)} {
		normalized := Normalize(input)
		if stderrors.Is(normalized, context.Canceled) {
			t.Fatal("nil cause became cancellation")
		}
		if stderrors.Is(normalized, InvalidInput("x", "y")) {
			t.Fatal("nil cause matched unrelated error")
		}
		var app *AppError
		if !stderrors.As(normalized, &app) || app.Code != ErrCodeInternal {
			t.Fatal("normalized identity lost")
		}
		if stderrors.Is(normalized, leaf) {
			t.Fatal("normalized absent error still unwraps to the typed-nil sentinel")
		}
		var diagnostic interface{ Timeout() bool }
		if stderrors.As(normalized, &diagnostic) {
			t.Fatal("unexpected cause type")
		}
	}
	if leaf.Error() != "<nil>" || leaf.Unwrap() != nil {
		t.Fatal("nil leaf is not safe")
	}
	if stderrors.Is(Internal(nil), leaf) {
		t.Fatal("an absent sentinel origin matched a typed-nil target")
	}
}
