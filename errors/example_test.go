package errors_test

import (
	"encoding/json"
	stderrors "errors"
	"fmt"

	"github.com/kbukum/gokit/errors"
)

func ExampleNotFound() {
	err := errors.NotFound("user", "42")
	fmt.Println(err)
	// Output: NOT_FOUND: The requested user was not found.
}

func ExampleAppError_WithDetails() {
	err := errors.NotFound("order", "abc").WithDetails(map[string]any{
		"tenant": "acme",
	})
	fmt.Println(err.Code, err.Details["tenant"])
	// Output: NOT_FOUND acme
}

func ExampleAppError_Unwrap() {
	cause := stderrors.New("connection refused")
	err := errors.ConnectionFailed("redis").WithCause(cause)

	if stderrors.Is(err, cause) {
		fmt.Println("cause preserved")
	}
	// Output: cause preserved
}

func ExampleCanceled_problemDetail() {
	err := errors.Canceled("search")
	body, _ := json.Marshal(err.ToProblemDetail())
	fmt.Println(string(body))
	// Output: {"type":"https://gokit.dev/errors/canceled","title":"Canceled","status":408,"detail":"The search operation was canceled.","code":"CANCELED","retryable":false,"details":{"operation":"search"}}
}
