package testutil

import "context"

// TokenValidator returns programmed typed claims or a diagnostic error for bearer-authentication tests.
type TokenValidator[T any] struct {
	Claims T
	Err    error
}

func (v TokenValidator[T]) ValidateToken(context.Context, string) (T, error) { return v.Claims, v.Err }
